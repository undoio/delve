package eval

import (
	"github.com/derekparker/delve/pkg/arch"
	pdwarf "github.com/derekparker/delve/pkg/dwarf"
	"github.com/derekparker/delve/pkg/memory"
)

// Scope is the scope for variable evaluation. Contains the thread,
// current location (PC), and canonical frame address.
type Scope struct {
	PC  uint64
	CFA int64
	Mem memory.ReadWriter

	dwarf *pdwarf.Dwarf
	arch  arch.Arch
	tls   uint64 // thread local storage
}

func NewScope(pc uint64, cfa int64, mem memory.ReadWriter, dwarf *pdwarf.Dwarf, arch arch.Arch, tls uint64) *Scope {
	return &Scope{
		PC:    pc,
		CFA:   cfa,
		Mem:   mem,
		dwarf: dwarf,
		arch:  arch,
		tls:   tls,
	}
}
