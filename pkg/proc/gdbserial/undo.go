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
	"syscall"

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
func undoGetInfo(conn *gdbConn) ([]string, error) {
	info, err := conn.undoCmd("get_info")
	if err != nil {
		return nil, err
	}
	splitter := func(c rune) bool {
		return c == ';' || c == ','
	}
	return strings.FieldsFunc(info, splitter), nil
}

// Fetch the mininum and maximum bbcounts of recorded history.
func undoGetLogExtent(conn *gdbConn) (uint64, uint64, error) {
	extent, err := conn.undoCmd("get_log_extent")
	if err != nil {
		return 0, 0, err
	}
	bbcounts := strings.Split(extent, ",")
	bbcount_min, err := strconv.ParseUint(bbcounts[0], 16, 64)
	if err != nil {
		return 0, 0, err
	}
	bbcount_max, err := strconv.ParseUint(bbcounts[1], 16, 64)
	if err != nil {
		return 0, 0, err
	}
	return bbcount_min, bbcount_max, nil
}

// Fetch whether the replay session is currently at the end of recorded history.
func undoAtEndOfHistory(conn *gdbConn) (bool, error) {
	info_fields, err := undoGetInfo(conn)
	if err != nil {
		return false, err
	}
	for _, value := range info_fields {
		if value == "has_exited" || value == "at_event_log_end" {
			return true, nil
		}
	}
	return false, nil
}

// Transform a stopPacket if necessary to represent the state of the replay session.
//
// Usually the packet will be passed through unaltered. Currently the only transformation
// implemented is modify a packet at the end of replay history to look like a SIGKILL, to be
// consistent with how RR would report this condition.
func undoHandleStopPacket(conn *gdbConn, sp stopPacket) (stopPacket, error) {
	// TODO: find a different way of indicating end of history as opposed to actual process
	// exit.
	//
	// TODO: find a different way of indicating the start of history (currently registers as a
	// "hardcoded breakpoint") - should we use the atstart flag that rr uses somehow?.

	at_end, err := undoAtEndOfHistory(conn)
	if err != nil {
		return stopPacket{}, err
	}

	if at_end {
		// Mirror the behaviour of rr, in which the server will send a fake SIGKILL
		// at the end of history.
		sp.sig = _SIGKILL
	}

	return sp, nil
}

// Fetch the exit code of the replay process (or zero, if not applicable) from the recording.
func undoGetExitCode(conn *gdbConn) (int, error) {
	exit_code := 0
	info_fields, err := undoGetInfo(conn)
	if err != nil {
		return 0, err
	}

	for idx, value := range info_fields {
		if value != "has_exited" {
			continue
		}

		// Exit status, encoded as hex, follows the has_exited string.
		exit_status, err := strconv.ParseInt(info_fields[idx+1], 16, 16)
		if err != nil {
			return 0, err
		}

		// Convert exit status into the form Delve usually reports - positive integer for a
		// normal exit, negative signal number if terminated by a signal.
		wait_status := syscall.WaitStatus(exit_status)
		if wait_status.Signaled() {
			exit_signal := wait_status.Signal()
			exit_code = -int(exit_signal)
		} else {
			exit_code = wait_status.ExitStatus()
		}
		break
	}

	return exit_code, nil
}

// Fetch a representation of the current time as a string.
func undoWhen(conn *gdbConn) (string, error) {
	resp, err := conn.undoCmd("get_time")
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
	bbcount_min, bbcount_max, err := undoGetLogExtent(conn)
	if err != nil {
		return "", err
	}

	progress := uint64(100)
	if bbcount_min != bbcount_max {
		progress = ((bbcount - bbcount_min) * 100) / (bbcount_max - bbcount_min)
	}

	result := fmt.Sprintf("[replaying %d%% %d:0x%x]", progress, bbcount, pc)
	return result, nil
}
