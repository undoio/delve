package proctl

import "fmt"

const (
	DR_RW_EXECUTE = 0x0 // Break on instruction execution
	DR_RW_WRITE   = 0x1 // Break on data write
	DR_RW_READ    = 0x3 // Break on data read

	DR_LEN_1 = (0x0 << 2) // 1-byte region watch or breakpoint
	DR_LEN_2 = (0x1 << 2) // 2-byte region watch
	DR_LEN_4 = (0x3 << 2) // 4-byte region watch
	DR_LEN_8 = (0x2 << 2) // 8-byte region watch (AMD64)

	DR_ENABLE_SIZE   = 2  // Two enable bits per register
	DR_CONTROL_SIZE  = 4  // Bits in DR7 per read/write and len field for each watchpoint
	DR_CONTROL_SHIFT = 16 // How many bits to skip in DR7
)

// Represents a single breakpoint. Stores information on the break
// point including the byte of data that originally was stored at that
// address.
type BreakPoint struct {
	FunctionName string
	File         string
	Line         int
	Addr         uint64
	OriginalData []byte
	ID           int
	Temp         bool
}

func (bp *BreakPoint) String() string {
	return fmt.Sprintf("Breakpoint %d at %#v %s:%d", bp.ID, bp.Addr, bp.File, bp.Line)
}

// Returned when trying to set a breakpoint at
// an address that already has a breakpoint set for it.
type BreakPointExistsError struct {
	file string
	line int
	addr uint64
}

func (bpe BreakPointExistsError) Error() string {
	return fmt.Sprintf("Breakpoint exists at %s:%d at %x", bpe.file, bpe.line, bpe.addr)
}

// InvalidAddressError represents the result of
// attempting to set a breakpoint at an invalid address.
type InvalidAddressError struct {
	address uint64
}

func (iae InvalidAddressError) Error() string {
	return fmt.Sprintf("Invalid address %#v\n", iae.address)
}

// Returns whether or not a breakpoint has been set for the given address.
func (dbp *DebuggedProcess) BreakpointExists(addr uint64) bool {
	for _, bp := range dbp.HWBreakPoints {
		if bp != nil && bp.Addr == addr {
			return true
		}
	}
	if _, ok := dbp.BreakPoints[addr]; ok {
		return true
	}
	return false
}

func (dbp *DebuggedProcess) newBreakpoint(fn, f string, l int, addr uint64, data []byte) *BreakPoint {
	dbp.breakpointIDCounter++
	return &BreakPoint{
		FunctionName: fn,
		File:         f,
		Line:         l,
		Addr:         addr,
		OriginalData: data,
		ID:           dbp.breakpointIDCounter,
	}
}

func (dbp *DebuggedProcess) setBreakpoint(tid int, addr uint64) (*BreakPoint, error) {
	var f, l, fn = dbp.GoSymTable.PCToLine(uint64(addr))
	if fn == nil {
		return nil, InvalidAddressError{address: addr}
	}
	if dbp.BreakpointExists(addr) {
		return nil, BreakPointExistsError{f, l, addr}
	}
	// Try and set a hardware breakpoint.
	for i, v := range dbp.HWBreakPoints {
		if v == nil {
			if err := setHardwareBreakpoint(i, tid, addr); err != nil {
				return nil, fmt.Errorf("could not set hardware breakpoint: %v", err)
			}
			dbp.HWBreakPoints[i] = dbp.newBreakpoint(fn.Name, f, l, addr, nil)
			return dbp.HWBreakPoints[i], nil
		}
	}
	// Fall back to software breakpoint. 0xCC is INT 3, software
	// breakpoint trap interrupt.
	thread := dbp.Threads[tid]
	originalData := make([]byte, 1)
	if _, err := readMemory(thread, uintptr(addr), originalData); err != nil {
		return nil, err
	}
	if _, err := writeMemory(thread, uintptr(addr), []byte{0xCC}); err != nil {
		return nil, err
	}
	dbp.BreakPoints[addr] = dbp.newBreakpoint(fn.Name, f, l, addr, originalData)
	return dbp.BreakPoints[addr], nil
}

// Sets a hardware breakpoint by setting the contents of the
// debug register `reg` with the address of the instruction
// that we want to break at. There are only 4 debug registers
// DR0-DR3. Debug register 7 is the control register.
func setHardwareBreakpoint(reg, tid int, addr uint64) error {
	if reg < 0 || reg > 3 {
		return fmt.Errorf("invalid debug register value")
	}

	var (
		drxmask   = uintptr((((1 << DR_CONTROL_SIZE) - 1) << uintptr(reg*DR_CONTROL_SIZE)) | (((1 << DR_ENABLE_SIZE) - 1) << uintptr(reg*DR_ENABLE_SIZE)))
		drxenable = uintptr(0x1) << uintptr(reg*DR_ENABLE_SIZE)
		drxctl    = uintptr(DR_RW_EXECUTE|DR_LEN_1) << uintptr(reg*DR_CONTROL_SIZE)
	)

	// Get current state
	dr7, err := getControlRegister(tid)
	if err != nil {
		return err
	}

	// If addr == 0 we are expected to disable the breakpoint
	if addr == 0 {
		dr7 &= ^drxmask
		return setControlRegister(tid, dr7)
	}

	// Error out if dr`reg` is already used
	if dr7&(0x3<<uint(reg*DR_ENABLE_SIZE)) != 0 {
		return fmt.Errorf("dr%d already enabled", reg)
	}

	// Set the debug register `reg` with the address of the
	// instruction we want to trigger a debug exception.
	if err := setDebugRegister(tid, reg, uintptr(addr)); err != nil {
		return err
	}

	// Clear dr`reg` flags
	dr7 &= ^drxmask
	// Enable dr`reg`
	dr7 |= (drxctl << DR_CONTROL_SHIFT) | drxenable

	// Set the debug control register. This
	// instructs the cpu to raise a debug
	// exception when hitting the address of
	// an instruction stored in dr0-dr3.
	return setControlRegister(tid, dr7)
}

// Clears a hardware breakpoint. Essentially sets
// the debug reg to 0 and clears the control register
// flags for that reg.
func clearHardwareBreakpoint(reg, tid int) error {
	return setHardwareBreakpoint(reg, tid, 0)
}

func (dbp *DebuggedProcess) clearBreakpoint(tid int, addr uint64) (*BreakPoint, error) {
	// Check for hardware breakpoint
	for i, bp := range dbp.HWBreakPoints {
		if bp == nil {
			continue
		}
		if bp.Addr == addr {
			dbp.HWBreakPoints[i] = nil
			if err := clearHardwareBreakpoint(i, tid); err != nil {
				return nil, err
			}
			return bp, nil
		}
	}
	// Check for software breakpoint
	if bp, ok := dbp.BreakPoints[addr]; ok {
		thread := dbp.Threads[tid]
		if _, err := writeMemory(thread, uintptr(bp.Addr), bp.OriginalData); err != nil {
			return nil, fmt.Errorf("could not clear breakpoint %s", err)
		}
		delete(dbp.BreakPoints, addr)
		return bp, nil
	}
	return nil, fmt.Errorf("No breakpoint currently set for %#v", addr)
}
