package debugger

import "debug/gosym"

// Location represents a location in a program.
type Location struct {
	PC   uint64
	File string
	Line int
	Fn   *gosym.Func
}
