package gdbserial_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/undoio/delve/pkg/proc"
	"github.com/undoio/delve/pkg/proc/gdbserial"
	protest "github.com/undoio/delve/pkg/proc/test"
)

func withUndoRecording(name string, t testing.TB, fn func(p *proc.Target, fixture protest.Fixture)) {
	fixture := protest.BuildFixture(name, 0)
	protest.MustHaveRecordingAllowed(t)
	if err := gdbserial.UndoIsAvailable(); err != nil {
		t.Skip("test skipped, Undo tools not found")
	}
	t.Log("recording")
	p, recording, err := gdbserial.UndoRecordAndReplay([]string{fixture.Path}, ".", true, []string{}, [3]string{})
	if err != nil {
		t.Fatal("Launch():", err)
	}
	t.Logf("replaying %q", recording)

	defer func() {
		p.Detach(true)
		if recording != "" {
			os.Remove(recording)
		}
	}()

	fn(p, fixture)
}

func TestUndoRestartAfterExit(t *testing.T) {
	protest.AllowRecording(t)
	withUndoRecording("testnextprog", t, func(p *proc.Target, fixture protest.Fixture) {
		setFunctionBreakpoint(p, t, "main.main")
		assertNoError(p.Continue(), t, "Continue")
		loc, err := p.CurrentThread().Location()
		assertNoError(err, t, "CurrentThread().Location()")
		err = p.Continue()
		if _, isexited := err.(proc.ErrProcessExited); err == nil || !isexited {
			t.Fatalf("program did not exit: %v", err)
		}

		assertNoError(p.Restart(""), t, "Restart")

		assertNoError(p.Continue(), t, "Continue (after restart)")
		loc2, err := p.CurrentThread().Location()
		assertNoError(err, t, "CurrentThread().Location() (after restart)")
		if loc2.Line != loc.Line {
			t.Fatalf("stopped at %d (expected %d)", loc2.Line, loc.Line)
		}
		err = p.Continue()
		if _, isexited := err.(proc.ErrProcessExited); err == nil || !isexited {
			t.Fatalf("program did not exit (after exit): %v", err)
		}
	})
}

func TestUndoRestartDuringStop(t *testing.T) {
	protest.AllowRecording(t)
	withUndoRecording("testnextprog", t, func(p *proc.Target, fixture protest.Fixture) {
		setFunctionBreakpoint(p, t, "main.main")
		assertNoError(p.Continue(), t, "Continue")
		loc, err := p.CurrentThread().Location()
		assertNoError(err, t, "CurrentThread().Location()")

		assertNoError(p.Restart(""), t, "Restart")

		assertNoError(p.Continue(), t, "Continue (after restart)")
		loc2, err := p.CurrentThread().Location()
		assertNoError(err, t, "CurrentThread().Location() (after restart)")
		if loc2.Line != loc.Line {
			t.Fatalf("stopped at %d (expected %d)", loc2.Line, loc.Line)
		}
		err = p.Continue()
		if _, isexited := err.(proc.ErrProcessExited); err == nil || !isexited {
			t.Fatalf("program did not exit (after exit): %v", err)
		}
	})
}

func TestUndoReverseBreakpointCounts(t *testing.T) {
	protest.AllowRecording(t)
	withUndoRecording("bpcountstest", t, func(p *proc.Target, fixture protest.Fixture) {
		endbp := setFileBreakpoint(p, t, fixture, 28)
		assertNoError(p.Continue(), t, "Continue()")
		loc, _ := p.CurrentThread().Location()
		if loc.PC != endbp.Addr {
			t.Fatalf("did not reach end of main.main function: %s:%d (%#x)", loc.File, loc.Line, loc.PC)
		}

		p.ClearBreakpoint(endbp.Addr)
		assertNoError(p.ChangeDirection(proc.Backward), t, "Switching to backward direction")
		bp := setFileBreakpoint(p, t, fixture, 12)
		startbp := setFileBreakpoint(p, t, fixture, 20)

	countLoop:
		for {
			assertNoError(p.Continue(), t, "Continue()")
			loc, _ := p.CurrentThread().Location()
			switch loc.PC {
			case startbp.Addr:
				break countLoop
			case bp.Addr:
				// ok
			default:
				t.Fatalf("unexpected stop location %s:%d %#x", loc.File, loc.Line, loc.PC)
			}
		}

		t.Logf("TotalHitCount: %d", bp.UserBreaklet().TotalHitCount)
		if bp.UserBreaklet().TotalHitCount != 200 {
			t.Fatalf("Wrong TotalHitCount for the breakpoint (%d)", bp.UserBreaklet().TotalHitCount)
		}

		if len(bp.UserBreaklet().HitCount) != 2 {
			t.Fatalf("Wrong number of goroutines for breakpoint (%d)", len(bp.UserBreaklet().HitCount))
		}

		for _, v := range bp.UserBreaklet().HitCount {
			if v != 100 {
				t.Fatalf("Wrong HitCount for breakpoint (%v)", bp.UserBreaklet().HitCount)
			}
		}
	})
}

func TestUndoCheckpoints(t *testing.T) {
	protest.AllowRecording(t)
	withUndoRecording("continuetestprog", t, func(p *proc.Target, fixture protest.Fixture) {
		// Continues until start of main.main, record output of 'when'
		bp := setFunctionBreakpoint(p, t, "main.main")
		assertNoError(p.Continue(), t, "Continue")
		when0, loc0 := getPosition(p, t)
		t.Logf("when0: %q (%#x) %x", when0, loc0.PC, p.CurrentThread().ThreadID())

		// Create a checkpoint and check that the list of checkpoints reflects this
		cpid, err := p.Checkpoint("checkpoint1")
		if cpid != 1 {
			t.Errorf("unexpected checkpoint id %d", cpid)
		}
		assertNoError(err, t, "Checkpoint")
		checkpoints, err := p.Checkpoints()
		assertNoError(err, t, "Checkpoints")
		if len(checkpoints) != 1 {
			t.Fatalf("wrong number of checkpoints %v (one expected)", checkpoints)
		}

		// Move forward with next, check that the output of 'when' changes
		assertNoError(p.Next(), t, "First Next")
		assertNoError(p.Next(), t, "Second Next")
		when1, loc1 := getPosition(p, t)
		t.Logf("when1: %q (%#x) %x", when1, loc1.PC, p.CurrentThread().ThreadID())
		if loc0.PC == loc1.PC {
			t.Fatalf("next did not move process %#x", loc0.PC)
		}
		if when0 == when1 {
			t.Fatalf("output of when did not change after next: %q", when0)
		}

		// Move back to checkpoint, check that the output of 'when' is the same as
		// what it was when we set the breakpoint
		p.Restart(fmt.Sprintf("c%d", cpid))
		g, _ := proc.FindGoroutine(p, 1)
		p.SwitchGoroutine(g)
		when2, loc2 := getPosition(p, t)
		t.Logf("when2: %q (%#x) %x", when2, loc2.PC, p.CurrentThread().ThreadID())
		if loc2.PC != loc0.PC {
			t.Fatalf("PC address mismatch %#x != %#x", loc0.PC, loc2.PC)
		}
		if when0 != when2 {
			t.Fatalf("output of when mismatched %q != %q", when0, when2)
		}

		// Move forward with next again, check that the output of 'when' matches
		assertNoError(p.Next(), t, "First Next")
		assertNoError(p.Next(), t, "Second Next")
		when3, loc3 := getPosition(p, t)
		t.Logf("when3: %q (%#x)", when3, loc3.PC)
		if loc3.PC != loc1.PC {
			t.Fatalf("PC address mismatch %#x != %#x", loc1.PC, loc3.PC)
		}
		if when3 != when1 {
			t.Fatalf("when output mismatch %q != %q", when1, when3)
		}

		// Delete breakpoint, move back to checkpoint then next twice and check
		// output of 'when' again
		err = p.ClearBreakpoint(bp.Addr)
		assertNoError(err, t, "ClearBreakpoint")
		p.Restart(fmt.Sprintf("c%d", cpid))
		g, _ = proc.FindGoroutine(p, 1)
		p.SwitchGoroutine(g)
		assertNoError(p.Next(), t, "First Next")
		assertNoError(p.Next(), t, "Second Next")
		when4, loc4 := getPosition(p, t)
		t.Logf("when4: %q (%#x)", when4, loc4.PC)
		if loc4.PC != loc1.PC {
			t.Fatalf("PC address mismatch %#x != %#x", loc1.PC, loc4.PC)
		}
		if when4 != when1 {
			t.Fatalf("when output mismatch %q != %q", when1, when4)
		}

		// Delete checkpoint, check that the list of checkpoints is updated
		assertNoError(p.ClearCheckpoint(cpid), t, "ClearCheckpoint")
		checkpoints, err = p.Checkpoints()
		assertNoError(err, t, "Checkpoints")
		if len(checkpoints) != 0 {
			t.Fatalf("wrong number of checkpoints %v (zero expected)", checkpoints)
		}
	})
}

func TestUndoIssue1376(t *testing.T) {
	// Backward Continue should terminate when it encounters the start of the process.
	protest.AllowRecording(t)
	withUndoRecording("continuetestprog", t, func(p *proc.Target, fixture protest.Fixture) {
		bp := setFunctionBreakpoint(p, t, "main.main")
		assertNoError(p.Continue(), t, "Continue (forward)")
		err := p.ClearBreakpoint(bp.Addr)
		assertNoError(err, t, "ClearBreakpoint")
		assertNoError(p.ChangeDirection(proc.Backward), t, "Switching to backward direction")
		assertNoError(p.Continue(), t, "Continue (backward)")
	})
}
