package gdbserial

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/undoio/delve/pkg/proc"
)

func serverFile() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "udbserver_x64", nil
	case "arm64":
		return "udbserver_arm64", nil
	case "386":
		return "udbserver_x32", nil
	default:
		return "", &ErrBackendUnavailable{}
	}
}

func UndoIsAvailable() error {
	server, err := serverFile()
	if err != nil {
		return err
	}

	cmds := []string{server, "live-record"}

	for _, cmd := range cmds {
		if _, err := exec.LookPath(cmd); err != nil {
			return &ErrBackendUnavailable{}
		}
	}
	return nil
}

func UndoRecord(cmd []string, wd string, quiet bool, redirects [3]string) (recording string, err error) {
	if err := UndoIsAvailable(); err != nil {
		return "", err
	}

	file, err := ioutil.TempFile("/tmp", "undo")
	if err != nil {
		return "", err
	}

	recording = file.Name()
	args := make([]string, 0)
	args = append(args, "-o", recording)
	args = append(args, cmd...)
	lrcmd := exec.Command("live-record", args...)
	var closefn func()
	// FIXME: pass quiet to openRedirects(), not false.
	lrcmd.Stdin, lrcmd.Stdout, lrcmd.Stderr, closefn, err = openRedirects(redirects, false)
	if err != nil {
		return "", err
	}
	if wd != "" {
		lrcmd.Dir = wd
	}
	lrcmd.Env = os.Environ()

	// Ignore failures from Run - it could be the target failing
	_ = lrcmd.Run()
	closefn()

	if isRecording, err := UndoIsRecording(recording); !isRecording {
		// Recording apparently failed to put anything in the file
		os.Remove(recording)
		if err == nil {
			err = fmt.Errorf("Recording failed")
		}
		return "", err
	}

	return recording, err
}

func UndoReplay(recording string, path string, quiet bool, debugInfoDirs []string) (tgt *proc.TargetGroup, err error) {
	if err := UndoIsAvailable(); err != nil {
		return nil, err
	}

	if isRecording, err := UndoIsRecording(recording); !isRecording || err != nil {
		if err == nil {
			err = fmt.Errorf("%s is not a LiveRecorder recording", recording)
		}
		return nil, err
	}

	port := unusedPort()

	args := make([]string, 0)
	args = append(args, "--load-file", recording, "--connect-port", port[1:])
	server, err := serverFile()
	if err != nil {
		return nil, err
	}
	servercmd := exec.Command(server, args...)

	if !quiet {
		servercmd.Env = os.Environ()
		// servercmd.Env = append(servercmd.Env, "UNDO_debug_filename=/dev/stderr")
		// servercmd.Env = append(servercmd.Env, "UNDO_debug_level=1")
		servercmd.Stdout = os.Stdout
		servercmd.Stderr = os.Stderr
	}

	if err := servercmd.Start(); err != nil {
		return nil, err
	}

	p := newProcess(servercmd.Process)
	p.tracedir = recording
	tgt, err = p.Dial(port, path, 0, debugInfoDirs, proc.StopAttached)
	if err != nil {
		servercmd.Process.Kill()
		return nil, err
	}

	// set to cause gdbserver.go to treat incoming signal numbers according
	// to the GDB mapping, not the Linux mapping (the binutils-gdb repo
	// defines the GDB mapping in include/gdb/signals.def)
	p.conn.isUndoServer = true
	return tgt, nil
}

// RecordAndReplay acts like calling Record and then Replay.
func UndoRecordAndReplay(cmd []string, wd string, quiet bool, debugInfoDirs []string, redirects [3]string) (tgt *proc.TargetGroup, recording string, err error) {
	recording, err = UndoRecord(cmd, wd, quiet, redirects)
	if err != nil || recording == "" {
		return nil, "", err
	}
	tgt, err = UndoReplay(recording, cmd[0], quiet, debugInfoDirs)
	return tgt, recording, err
}

func UndoIsRecording(recordingFile string) (result bool, err error) {
	marker := []byte("HD\x10\x00\x00\x00UndoDB recording")

	f, err := os.Open(recordingFile)
	if err != nil {
		return false, err
	}
	defer f.Close()

	data := make([]byte, len(marker))
	c, err := f.Read(data)
	if err != nil || c != len(marker) {
		return false, err
	}

	return bytes.Equal(marker, data), nil
}

// Fetch the output of a udbserver get_info command, split on ; and , characters.
//
// This is not (currently) implementing a proper parse of the data returned, just making it more
// convenient to search.
func undoGetInfo(conn gdbConn) []string {
	info, err := conn.undoCmd("get_info")
	if err != nil {
		panic("Failed to retrieve Undo stop info.")
	}
	splitter := func(c rune) bool {
		return c == ';' || c == ','
	}
	return strings.FieldsFunc(info, splitter)
}

func undoGetLogExtent(p *gdbProcess) (uint64, uint64) {
	extent, err := p.conn.undoCmd("get_log_extent")
	if err != nil {
		panic("Failed to retrieve Undo log extent.")
	}
	bbcounts := strings.Split(extent, ",")
	bbcount_min, err := strconv.ParseUint(bbcounts[0], 16, 64)
	if err != nil {
		panic("Failed to parse Undo log minimum.")
	}
	bbcount_max, err := strconv.ParseUint(bbcounts[1], 16, 64)
	if err != nil {
		panic("Failed to parse Undo log maximum.")
	}
	return bbcount_min, bbcount_max
}

// Fetch whether the replay session is currently at the end of recorded history.
func UndoAtEndOfHistory(p *gdbProcess) bool {
	info_fields := undoGetInfo(p.conn)
	for _, value := range info_fields {
		if value == "has_exited" || value == "at_event_log_end" {
			return true
		}
	}
	return false
}

// Fetch the process exit code (or zero, if not applicable) from the recording.
func UndoGetExitCode(p *gdbProcess) int {
	exit_code := int8(0)
	info_fields := undoGetInfo(p.conn)

	for idx, value := range info_fields {
		if value != "has_exited" {
			continue
		}

		// Exit status, encoded as hex, follows the has_exited string.
		exit_status, err := strconv.ParseInt(info_fields[idx+1], 16, 16)
		if err != nil {
			panic("Invalid exit code.")
		}

		// Convert exit status into the form Delve usually reports - positive integer for a
		// normal exit, negative signal number if terminated by a signal.
		if exit_status&0x7f != 0 {
			exit_code = -int8(exit_status & 0x7f)
		} else {
			exit_code = int8(exit_status >> 8)
		}
		break
	}

	return int(exit_code)
}

func undoWhen(p *gdbProcess) (string, error) {
	resp, err := p.conn.undoCmd("get_time")
	if err != nil {
		return "", err
	}

	// We have received a comma-separated list of hex numbers.
	time_parts := strings.Split(resp, ",")

	// First component is bbcount.
	bbcount, err := strconv.ParseUint(time_parts[0], 16, 64)
	if err != nil {
		return "", err
	}

	// Second component is PC.
	pc, err := strconv.ParseUint(time_parts[1], 16, 64)
	if err != nil {
		return "", err
	}

	// Calculate our percentage through available history.
	bbcount_min, bbcount_max := undoGetLogExtent(p)
	progress := ((bbcount - bbcount_min) * 100) / (bbcount_max - bbcount_min)

	result := fmt.Sprintf("[replaying %d%% %d:0x%x]", progress, bbcount, pc)
	return result, nil
}
