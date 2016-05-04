package goroutine

import (
	"fmt"
	"strings"

	pdwarf "github.com/derekparker/delve/pkg/dwarf"
	"github.com/derekparker/delve/pkg/location"
	"github.com/derekparker/delve/pkg/memory"
	"github.com/derekparker/delve/pkg/stack"
)

const (
	chanRecv = "chan receive"
	chanSend = "chan send"

	// G status, from: $GOROOT/src/runtime/runtime2.go
	Gidle           uint64 = iota // 0
	Grunnable                     // 1 runnable and on a run queue
	Grunning                      // 2
	Gsyscall                      // 3
	Gwaiting                      // 4
	GmoribundUnused               // 5 currently unused, but hardcoded in gdb scripts
	Gdead                         // 6
	Genqueue                      // 7 Only the Gscanenqueue is used.
	Gcopystack                    // 8 in this state when newstack is moving the stack
)

// G represents a runtime G (goroutine) structure (at least the
// fields that Delve is interested in).
type G struct {
	ID     int    // Goroutine ID
	PC     uint64 // PC of goroutine when it was parked.
	SP     uint64 // SP of goroutine when it was parked.
	GoPC   uint64 // PC of 'go' statement that created this goroutine.
	Status uint64
	// PC of entry to top-most deferred function.
	DeferPC    uint64
	WaitReason string // Reason for goroutine being parked.

	// Information on goroutine location.
	CurrentLoc location.Location

	ThreadID int

	Mem   memory.ReadWriter
	Dwarf *pdwarf.Dwarf

	// Thread that this goroutine is currently allocated to
	// thread *proc.Thread
}

// NoGError returned when a G could not be found
// for a specific thread.
type NoGError struct {
	Tid int
}

func (ng NoGError) Error() string {
	return fmt.Sprintf("no G executing on thread %d", ng.Tid)
}

// ChanRecvBlocked returns whether the goroutine is blocked on
// a channel read operation.
func (g *G) ChanRecvBlocked() bool {
	return g.WaitReason == chanRecv
}

// ChanRecvReturnAddr returns the address of the return from a channel read.
func (g *G) ChanRecvReturnAddr() (uint64, error) {
	locs, err := stack.Trace(4, g.PC, g.SP, g.Dwarf, g.Mem)
	if err != nil {
		return 0, err
	}
	topLoc := locs[len(locs)-1]
	return topLoc.Current.PC, nil
}

// UserCurrent returns the location the users code is at,
// or was at before entering a runtime function.
func (g *G) UserCurrent() location.Location {
	it := stack.NewIterator(g.PC, g.SP, g.Dwarf, g.Mem)
	for it.Next() {
		frame := it.Frame()
		if frame.Call.Fn != nil {
			name := frame.Call.Fn.Name
			if (strings.Index(name, ".") >= 0) && (!strings.HasPrefix(name, "runtime.") || isExportedRuntime(name)) {
				return frame.Call
			}
		}
	}
	return g.CurrentLoc
}

// Go returns the location of the 'go' statement
// that spawned this goroutine.
func (g *G) Go() location.Location {
	f, l, fn := g.Dwarf.PCToLine(g.GoPC)
	return location.New(g.GoPC, f, l, fn)
}

func (g *G) Dead() bool {
	return g.Status == Gdead
}

// From $GOROOT/src/runtime/traceback.go:597
// isExportedRuntime reports whether name is an exported runtime function.
// It is only for runtime functions, so ASCII A-Z is fine.
func isExportedRuntime(name string) bool {
	const n = len("runtime.")
	return len(name) > n && name[:n] == "runtime." && 'A' <= name[n] && name[n] <= 'Z'
}
