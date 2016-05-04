package location

import (
	"debug/gosym"
	"fmt"
)

// Location represents the location of a thread.
// Holds information on the current instruction
// address, the source file:line, and the function.
type Location struct {
	PC   uint64
	File string
	Line int
	Fn   *gosym.Func
}

func New(pc uint64, f string, l int, fn *gosym.Func) Location {
	return Location{PC: pc, File: f, Line: l, Fn: fn}
}

func (l *Location) String() string {
	s := fmt.Sprintf("%s:%d (%#v)", l.File, l.Line, l.PC)
	if l.Fn != nil {
		s = fmt.Sprintf("%s %s", s, l.Fn.Name)
	}
	return s
}
