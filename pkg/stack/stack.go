package stack

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/derekparker/delve/pkg/dwarf"
	"github.com/derekparker/delve/pkg/dwarf/frame"
	"github.com/derekparker/delve/pkg/location"
	"github.com/derekparker/delve/pkg/memory"
)

// NoReturnAddr is returned when return address
// could not be found during stack tracae.
type NoReturnAddr struct {
	fn string
}

func (nra NoReturnAddr) Error() string {
	return fmt.Sprintf("could not find return address for %s", nra.fn)
}

// Frame represents a frame in a system stack.
type Frame struct {
	// Address the function above this one on the call stack will return to.
	Current location.Location
	// Address of the call instruction for the function above on the call stack.
	Call location.Location
	CFA  int64
	Ret  uint64
}

// ReturnAddress returns the return address of the function
// this thread is executing.
func ReturnAddress(pc, sp uint64, dwarf *dwarf.Dwarf, mem memory.ReadWriter) (uint64, error) {
	locations, err := Trace(2, pc, sp, dwarf, mem)
	if err != nil {
		return 0, err
	}
	if len(locations) < 2 {
		return 0, NoReturnAddr{locations[0].Current.Fn.BaseName()}
	}
	return locations[1].Current.PC, nil
}

// Trace returns the stack trace for thread.
// Note the locations in the array are return addresses not call addresses.
func Trace(depth int, pc, sp uint64, dwarf *dwarf.Dwarf, mem memory.ReadWriter) ([]Frame, error) {
	return NewIterator(pc, sp, dwarf, mem).unwind(depth)
}

// NullAddrError is an error for a null address.
type NullAddrError struct{}

func (n NullAddrError) Error() string {
	return "NULL address"
}

// stackIterator holds information
// required to iterate and walk the program
// stack.
type Iterator struct {
	pc, sp uint64
	top    bool
	atend  bool
	frame  Frame
	dwarf  *dwarf.Dwarf
	mem    memory.ReadWriter
	err    error
}

func NewIterator(pc, sp uint64, dwarf *dwarf.Dwarf, mem memory.ReadWriter) *Iterator {
	return &Iterator{pc: pc, sp: sp, top: true, dwarf: dwarf, mem: mem, err: nil, atend: false}
}

// Next points the iterator to the next stack frame.
func (it *Iterator) Next() bool {
	if it.err != nil || it.atend {
		return false
	}
	it.frame, it.err = frameInfo(it.pc, it.sp, it.top, it.dwarf, it.mem)
	if it.err != nil {
		if _, nofde := it.err.(*frame.NoFDEForPCError); nofde && !it.top {
			it.frame = Frame{Current: location.New(it.pc, "?", -1, nil), Call: location.New(it.pc, "?", -1, nil), CFA: 0, Ret: 0}
			it.atend = true
			it.err = nil
			return true
		}
		return false
	}

	if it.frame.Current.Fn == nil {
		if it.top {
			it.err = fmt.Errorf("PC not associated to any function")
		}
		return false
	}

	if it.frame.Ret <= 0 {
		it.atend = true
		return true
	}
	// Look for "top of stack" functions.
	if it.frame.Current.Fn.Name == "runtime.goexit" || it.frame.Current.Fn.Name == "runtime.rt0_go" || it.frame.Current.Fn.Name == "runtime.mcall" {
		it.atend = true
		return true
	}

	it.top = false
	it.pc = it.frame.Ret
	it.sp = uint64(it.frame.CFA)
	return true
}

// Frame returns the frame the iterator is pointing at.
func (it *Iterator) Frame() Frame {
	if it.err != nil {
		panic(it.err)
	}
	return it.frame
}

// Err returns the error encountered during stack iteration.
func (it *Iterator) Err() error {
	return it.err
}

func (it *Iterator) unwind(depth int) ([]Frame, error) {
	if depth < 0 {
		return nil, errors.New("negative maximum stack depth")
	}
	frames := make([]Frame, 0, depth+1)
	for it.Next() {
		frames = append(frames, it.Frame())
		if len(frames) >= depth+1 {
			break
		}
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return frames, nil
}

func frameInfo(pc, sp uint64, top bool, dwarf *dwarf.Dwarf, mem memory.ReadWriter) (Frame, error) {
	f, l, fn := dwarf.PCToLine(pc)
	fde, err := dwarf.Frame.FDEForPC(pc)
	if err != nil {
		return Frame{}, err
	}
	spoffset, retoffset := fde.ReturnAddressOffset(pc)
	cfa := int64(sp) + spoffset

	retaddr := uint64(cfa + retoffset)
	if retaddr == 0 {
		return Frame{}, NullAddrError{}
	}
	data, err := mem.Read(retaddr, mem.Arch().PtrSize())
	if err != nil {
		return Frame{}, err
	}
	r := Frame{Current: location.New(pc, f, l, fn), CFA: cfa, Ret: binary.LittleEndian.Uint64(data)}
	if !top {
		r.Call.File, r.Call.Line, r.Call.Fn = dwarf.PCToLine(pc - 1)
		r.Call.PC, _, _ = dwarf.LineToPC(r.Call.File, r.Call.Line)
	} else {
		r.Call = r.Current
	}
	return r, nil
}
