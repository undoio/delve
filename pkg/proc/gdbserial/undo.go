package gdbserial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/undoio/delve/pkg/proc"
)

// State relating to an Undo "session" - used to correctly interpret and handle time-travel
// operations on a gdbConn when running with the Undo backend.
//
// The current checkpoints are persisted to disk in an "Undo session file" via the save() method.
// They are restored via the load() method.
type undoSession struct {
	checkpointNextId int                     // For allocating checkpoint IDs
	checkpoints      map[int]proc.Checkpoint // Map checkpoint IDs to Delve's proc.Checkpoint
	volatile         bool                    // Is the Undo connection currently in volatile mode?
}

// Create a new undoSession structure.
func newUndoSession() *undoSession {
	return &undoSession{
		checkpointNextId: 1,
		checkpoints:      make(map[int]proc.Checkpoint),
		volatile:         false,
	}
}

// undoCmd executes a vUDB command
func undoCmd(conn *gdbConn, args ...string) (string, error) {
	if len(args) == 0 {
		panic("must specify at least one argument for undoCmd")
	}
	conn.outbuf.Reset()
	fmt.Fprint(&conn.outbuf, "$vUDB")
	for _, arg := range args {
		fmt.Fprint(&conn.outbuf, ";", arg)
	}
	resp, err := conn.exec(conn.outbuf.Bytes(), "undoCmd")
	if err != nil {
		return "", err
	}
	return string(resp), nil
}

// Validate a checkpoint note to ensure easy interopability with UDB bookmarks.
// Returns nil (no error) if a checkpoint is validated successfully.
func validateCheckpointNote(where string) error {
	if where == "" {
		panic("checkpoint note expectedly empty.")
	}

	// Perform matching checking to UDB's bookmark creation code, to ensure Delve notes
	// translate properly into UDB bookmarks when we save the session.
	//
	// (compare Bookmarks._verify_name() in the UDB Python code)
	firstChar := where[0:1]
	if firstChar == " " {
		return errors.New("checkpoint note must not start with a space.")
	} else if _, err := strconv.Atoi(firstChar); err == nil {
		return errors.New("checkpoint note must not start with a digit.")
	} else if firstChar == "," || firstChar == "-" || firstChar == "$" {
		return fmt.Errorf("checkpoint note must not start with character: %c", where[0])
	} else {
		firstWord := strings.Split(where, " ")[0]
		reserved := []string{"annotation", "bookmark", "end", "event", "inferior", "pc",
			"redo", "start", "time", "undo", "wallclock"}
		for _, reservedWord := range reserved {
			if firstWord == reservedWord {
				return fmt.Errorf("checkpoint note must not start with reserved word: %s", reservedWord)
			}
		}
	}

	return nil
}

// Create a Delve checkpoint structure at the current time, with the supplied note.
func (uc *undoSession) createCheckpoint(conn *gdbConn, where string) (int, error) {
	err := validateCheckpointNote(where)
	if err != nil {
		return -1, err
	}
	cpid := uc.checkpointNextId
	uc.checkpointNextId++
	when, err := undoCmd(conn, "get_time")
	if err != nil {
		return -1, err
	}
	uc.checkpoints[cpid] = proc.Checkpoint{ID: cpid, When: when, Where: where}
	uc.save(conn)
	return cpid, nil
}

// Look up a Delve checkpoint structure by name.
func (uc *undoSession) lookupCheckpoint(pos string) (proc.Checkpoint, error) {
	if len(pos) == 0 {
		panic("empty checkpoint name")
	}
	if pos[0] != 'c' {
		panic("invalid checkpoint name")
	}
	cpid, _ := strconv.Atoi(pos[1:])
	checkpoint, exists := uc.checkpoints[cpid]
	if !exists {
		return proc.Checkpoint{}, errors.New("checkpoint not found")
	}
	return checkpoint, nil
}

// Delete a Delve checkpoint structure from our tracking.
func (uc *undoSession) deleteCheckpoint(conn *gdbConn, id int) {
	delete(uc.checkpoints, id)
	uc.save(conn)
}

// Fetch all Delve checkpoint structures and return an array for user display (with the When field
// rewritten in human readable form).
func (uc *undoSession) getCheckpoints() ([]proc.Checkpoint, error) {
	r := make([]proc.Checkpoint, 0, len(uc.checkpoints))
	for _, cp := range uc.checkpoints {
		// Convert the internal representation of time (which is based on the serial
		// protocol level representation) to a human-readable version for display.
		bbcount, pc, err := undoParseServerTime(cp.When)
		if err != nil {
			return nil, err
		}
		cp.When = undoTimeString(bbcount, pc)
		r = append(r, cp)
	}
	return r, nil
}

// Represents a single serialised bookmark in our session file format.
type bookmarkTime struct {
	Bbcount uint64 `json:"bbcount"`
	Pc      uint64 `json:"pc"`
}

// Represents the overall structure of our session file format.
type session struct {
	Bookmarks map[string]bookmarkTime `json:"bookmarks"`
}

// Get the path to the UDB session file for the current recording.
func getSessionPath(conn *gdbConn) (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	recording_ids, err := undoCmd(conn, "get_recording_ids")
	if err != nil {
		return "", err
	}
	uuids := strings.Split(recording_ids, ";")
	if len(uuids) != 3 || uuids[1] == "" {
		panic("unexpected response from get_recording_ids")
	}

	// This directory stores sessions.
	xdg_data_dir, present := os.LookupEnv("XDG_DATA_HOME")
	if !present {
		xdg_data_dir = filepath.Join(user.HomeDir, ".local", "share")
	}
	undo_sessions_dir := filepath.Join(xdg_data_dir, "undo", "sessions")

	err = os.MkdirAll(undo_sessions_dir, 0755)
	if err != nil {
		return "", err
	}

	file := filepath.Join(undo_sessions_dir, string(uuids[1])+".json")

	return file, nil
}

// Load the UDB session file (if it exists) for the current recording.
func (uc *undoSession) load(conn *gdbConn) error {
	path, err := getSessionPath(conn)
	if err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var s session
	err = decoder.Decode(&s)
	if err != nil {
		return err
	}

	// Clear out the session data.
	*uc = *newUndoSession()

	// Translate the loaded Undo bookmarks into Delve checkpoints.
	for name, position := range s.Bookmarks {
		cpid := uc.checkpointNextId
		uc.checkpointNextId++
		uc.checkpoints[cpid] = proc.Checkpoint{
			ID:    cpid,
			When:  fmt.Sprintf("%x,%x", position.Bbcount, position.Pc),
			Where: name,
		}
	}

	return err
}

// Save the session file for the current recording.
func (uc *undoSession) save(conn *gdbConn) error {
	// Translate Delve checkpoints into Undo bookmarks.
	var s session
	s.Bookmarks = make(map[string]bookmarkTime)

	// Local copy of the checkpoints.
	var checkpoints []proc.Checkpoint
	for _, cp := range uc.checkpoints {
		checkpoints = append(checkpoints, cp)
	}
	// Sort the checkpoints by descending note length - this is to avoid adding a suffix to a
	// entries that we've already added a suffix to. e.g. if we've previously saved this session
	// with a duplicated checkpoint note called "test" then we'll have extended one to
	// "test-0". If the user adds "test" again, we want to rename that to "test-1" rather than
	// creating a "test-0-0".
	sort.Slice(checkpoints, func(i, j int) bool {
		// This is a "Less" function that sorts in descending order of string length.
		return len(checkpoints[i].Where) > len(checkpoints[j].Where)
	})

	// Iterate through sorted checkpoints to eliminate duplicates.
	for _, cp := range checkpoints {
		// Ensure that notes are made unique before saving - UDB expects bookmark names to
		// be unique.
		base_name := cp.Where
		name := base_name
		for i := 0; s.Bookmarks[name] != (bookmarkTime{}); i++ {
			name = fmt.Sprintf("%s-%d", base_name, i)
		}

		var time bookmarkTime
		_, err := fmt.Sscanf(cp.When, "%x,%x", &time.Bbcount, &time.Pc)
		if err != nil {
			return err
		}
		s.Bookmarks[name] = time
	}

	path, err := getSessionPath(conn)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	err = encoder.Encode(&s)
	if err != nil {
		return err
	}

	return err
}

// Transform a user-specified time into a canonical form. The returned string has been validated
// (unknown checkpoint IDs, misspelt magic values and incorrectly formatted times will be rejected)
// and is suitable for passing to travelToTime.
func (uc *undoSession) resolveUserTime(pos string) (string, error) {
	// Validate and transform input.
	//
	// We will accept:
	//   start | end - magic values for getting to the extremes of history.
	//   cN          - a checkpoint name (a "c" character followed by an integer ID)
	//   BBCOUNT     - an Undo bbcount as a decimal integer (with or without comma-separated
	//                 grouping of digits).
	//   BBCOUNT:PC  - an Undo bbcount, as above, followed by a colon and then a program
	//                 counter value in hex (with leading 0x).
	if pos == "start" || pos == "end" {
		// Special case values - valid with no extra checking.
	} else if len(pos) > 1 && pos[:1] == "c" {
		// Validate a checkpoint ID.
		checkpoint, err := uc.lookupCheckpoint(pos)
		if err != nil {
			return "", err
		}
		pos = checkpoint.When
	} else if pos != "" {
		// Validate a potential bbcount or precise time.
		pos = strings.ReplaceAll(pos, ",", "")
		var bbcount, pc uint64
		var err error
		if strings.Contains(pos, ":") {
			_, err = fmt.Sscanf(pos, "%d:0x%x\n", &bbcount, &pc)
		} else if _, err = fmt.Sscanf(pos, "%d\n", &bbcount); err == nil {
			// It's a valid bbcount.
			pc = 0
		}

		if err != nil {
			return "", errors.New("could not parse time or checkpoint argument to restart.")
		}

		// A representation of the current time, as used by the udbserver serial protocol.
		// This matches the format returned by vUDB;get_time and can be used as an argument
		// to vUDB;goto_time.
		pos = fmt.Sprintf("%x;%x", bbcount, pc)
	}

	return pos, nil
}

// Move the replay process to the a point in time. The "pos" argument should be obtained by calling
// resolveUserTime to ensure that it is valid.
func (uc *undoSession) travelToTime(conn *gdbConn, pos string) error {
	var args []string
	switch pos {
	case "start", "":
		// Find the actual min BB count.
		minBbCount, _, err := undoGetLogExtent(conn)
		if err != nil {
			return err
		}
		args = []string{"goto_time", fmt.Sprint(minBbCount), "0"}
	case "end":
		args = []string{"goto_record_mode"}
	default:
		args = []string{"goto_time", pos}
	}
	_, err := undoCmd(conn, args...)
	return err
}

// Activate volatile mode.
// On success, returns a callback that can be used to deactivate volatile mode (and a nil error).
// The deactivate callback should be used before volatile is next activated, since volatile mode
// does not support nesting.
func (uc *undoSession) activateVolatile(conn *gdbConn) (func(), error) {
	if uc.volatile {
		panic("tried to activate volatile mode when already active.")
	}
	_, err := undoCmd(conn, "set_debuggee_volatile", "1")
	if err != nil {
		return nil, err
	}
	uc.volatile = true
	return func() {
		uc.volatile = false
		_, _ = undoCmd(conn, "set_debuggee_volatile", "0")
	}, nil
}

// Callback before Delve begins a continue-type operation.
// Used to ensure our progress indicators are active and ready to start. Returns an error of nil on
// success.
func (uc *undoSession) continuePre(conn *gdbConn) error {
	if uc.volatile {
		return nil
	}
	// Clear interrupt (and enable progress indication)
	_, err := undoCmd(conn, "clear_interrupt")
	return err
}

// Callback after Delve finishes a continue-type operation.
// Used to ensure our progress indicators are reset. Returns an error of nil on success.
func (uc *undoSession) continuePost(conn *gdbConn) error {
	if uc.volatile {
		return nil
	}
	_, reset_err := undoCmd(conn, "reset_progress_indicator")
	if reset_err != nil {
		conn.log.Errorf("Error %s from reset_progress_indicator", reset_err)
	}
	return reset_err
}

// Callback before Delve starts a restart-type operation.
// Used to ensure our progress indicators are active and ready to start. Returns an error of nil on
// success.
func (uc *undoSession) restartPre(conn *gdbConn) error {
	if uc.volatile {
		// We should only be in volatile mode during an inferior call, so this case
		// should not be possible.
		panic("attempted to restart in volatile mode.")
	}
	// Clear interrupt (and enable progress indication)
	_, err := undoCmd(conn, "clear_interrupt")
	return err
}

// Callback after Delve finishes a restart-type operation.
// Used to ensure our progress indicators are reset. Returns an error of nil on success.
func (uc *undoSession) restartPost(conn *gdbConn) error {
	if uc.volatile {
		// Restart should not change our volatile mode state.
		panic("in volatile mode after restart.")
	}
	_, reset_err := undoCmd(conn, "reset_progress_indicator")
	if reset_err != nil {
		conn.log.Errorf("Error %s from reset_progress_indicator", reset_err)
	}
	return reset_err
}

// Get the UDB server filename for the current architecture.
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

// Get the full path to the UDB server for the current architecture.
func serverPath() (string, error) {
	udb_path, err := exec.LookPath("udb")
	if err != nil {
		return "", err
	}

	udb_path_abs, err := filepath.EvalSymlinks(udb_path)
	if err != nil {
		return "", err
	}

	server_file, err := serverFile()
	if err != nil {
		return "", err
	}

	udb_dir_abs := filepath.Dir(udb_path_abs)
	cmd_path := filepath.Join(udb_dir_abs, server_file)

	return cmd_path, nil
}

func UndoIsAvailable() error {
	server, err := serverPath()
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
			err = fmt.Errorf("recording failed")
		}
		return "", err
	}

	return recording, err
}

func UndoReplay(recording string, path string, quiet bool, debugInfoDirs []string, cmdline string) (tgt *proc.TargetGroup, err error) {
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
	server, err := serverPath()
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
	tgt, err = p.Dial(port, path, cmdline, 0, debugInfoDirs, proc.StopAttached)
	if err != nil {
		servercmd.Process.Kill()
		return nil, err
	}

	// Create storage for Undo-related state.
	//
	// This being non-nil indicates the use of an Undo backend, which selects alternative
	// implementations for various functions and handles certain events (such as
	// gdbserial stop packets) differently.
	p.conn.undoSession = newUndoSession()

	// Load the session details if possible (discarding errors, which are non-fatal).
	_ = p.conn.undoSession.load(&p.conn)

	return tgt, nil
}

// RecordAndReplay acts like calling Record and then Replay.
func UndoRecordAndReplay(cmd []string, wd string, quiet bool, debugInfoDirs []string, redirects [3]string) (tgt *proc.TargetGroup, recording string, err error) {
	recording, err = UndoRecord(cmd, wd, quiet, redirects)
	if err != nil || recording == "" {
		return nil, "", err
	}
	tgt, err = UndoReplay(recording, cmd[0], quiet, debugInfoDirs, strings.Join(cmd, " "))
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
	info, err := undoCmd(conn, "get_info")
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
	extent, err := undoCmd(conn, "get_log_extent")
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

// Print a bbcount and PC pair in standard Undo time notiation.
func undoTimeString(bbcount uint64, pc uint64) string {
	var bbcount_groups []string

	// Chop 3 digits at a time from the low-order end of the bbcount string.
	var bbcount_rem uint64
	for bbcount_rem = bbcount; bbcount_rem > 1000; bbcount_rem = bbcount_rem / 1000 {
		// Format the group with leading zeros.
		group := fmt.Sprintf("%03d", bbcount_rem%1000)
		bbcount_groups = append([]string{group}, bbcount_groups...)
	}
	// Finally, add the highest-order group, which has no leading zeros.
	bbcount_groups = append([]string{fmt.Sprintf("%d", bbcount_rem)}, bbcount_groups...)

	// Return the comma-separated whole.
	return fmt.Sprintf("%s:0x%x", strings.Join(bbcount_groups, ","), pc)
}

// Parse the udbserver serial-level representation of a time into bbcount and PC.
func undoParseServerTime(resp string) (uint64, uint64, error) {
	// We have received a comma-separated list of hex numbers.
	time_parts := strings.Split(resp, ",")

	// First component is bbcount.
	bbcount, err := strconv.ParseUint(time_parts[0], 16, 64)
	if err != nil {
		return 0, 0, err
	}

	// Second component is PC.
	pc, err := strconv.ParseUint(time_parts[1], 16, 64)
	if err != nil {
		return 0, 0, err
	}

	return bbcount, pc, nil
}

// Fetch a representation of the current time as a string.
func undoWhen(conn *gdbConn) (string, error) {
	resp, err := undoCmd(conn, "get_time")
	if err != nil {
		return "", err
	}

	bbcount, pc, err := undoParseServerTime(resp)
	if err != nil {
		return "", err
	}

	// Calculate our percentage through available history.
	bbcount_min, bbcount_max, err := undoGetLogExtent(conn)
	if err != nil {
		return "", err
	}

	history_perc := uint64(100)
	if bbcount_min != bbcount_max {
		history_perc = ((bbcount - bbcount_min) * 100) / (bbcount_max - bbcount_min)
	}

	history_perc_fmt := fmt.Sprintf("%d%%", history_perc)
	result := fmt.Sprintf("[replaying %s %s]", history_perc_fmt, undoTimeString(bbcount, pc))
	return result, nil
}
