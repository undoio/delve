package gdbserial

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
)

func serverFile() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "undodb-server_x64", nil
	case "arm64":
		return "undodb-server_arm64", nil
	case "386":
		return "undodb-server_x32", nil
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

func UndoRecord(cmd []string, wd string, quiet bool) (recording string, err error) {
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

	if !quiet {
		lrcmd.Env = os.Environ()
		// Setting UNDO_debug_level alone is not enough
		// to get all debug information out of undodb.
		// Set an empty UNDO_debug_filename to get all debug
		// output on stderr.
		lrcmd.Env = append(lrcmd.Env, "UNDO_debug_filename=")
		lrcmd.Env = append(lrcmd.Env, "UNDO_debug_level=1")
		lrcmd.Stdout = os.Stdout
		lrcmd.Stderr = os.Stderr
	}

	// Ignore failures from Run - it could be the target failing
	_ = lrcmd.Run()

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

func UndoReplay(recording string, path string, quiet bool, debugInfoDirs []string) (p *Process, err error) {
	if err := UndoIsAvailable(); err != nil {
		return nil, err
	}

	if isRecording, err := UndoIsRecording(recording); !isRecording || err != nil {
		if err == nil {
			err = fmt.Errorf("%s is not an UndoDB recording", recording)
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
		// Setting UNDO_debug_level alone is not enough
		// to get all debug information out of undodb.
		// Set an empty UNDO_debug_filename to get all debug
		// output on stderr.
		servercmd.Env = append(servercmd.Env, "UNDO_debug_filename=")
		servercmd.Env = append(servercmd.Env, "UNDO_debug_level=1")
		servercmd.Stdout = os.Stdout
		servercmd.Stderr = os.Stderr
	}

	if err := servercmd.Start(); err != nil {
		return nil, err
	}

	p = New(servercmd.Process)
	p.tracedir = recording
	err = p.Dial(port, path, 0, debugInfoDirs)
	if err != nil {
		servercmd.Process.Kill()
		return nil, err
	}

	// set to cause gdbserver.go to treat incoming signal numbers according to the GDB
	// mapping, not the Linux mapping (see signal2native and native2signal in gdbserial.cpp)
	p.conn.isUndoServer = true
	return p, nil
}

// RecordAndReplay acts like calling Record and then Replay.
func UndoRecordAndReplay(cmd []string, wd string, quiet bool, debugInfoDirs []string) (p *Process, recording string, err error) {
	recording, err = UndoRecord(cmd, wd, quiet)
	if err != nil || recording == "" {
		return nil, "", err
	}
	p, err = UndoReplay(recording, cmd[0], quiet, debugInfoDirs)
	return p, recording, err
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
