package proc

import (
	"syscall"
	"unsafe"

	sys "golang.org/x/sys/unix"
)

// PtraceAttach executes the sys.PtraceAttach call.
func PtraceAttach(pid int) error {
	var err error
	execOnPtraceThread(func() { err = sys.PtraceAttach(pid) })
	return err
}

// PtraceDetach calls ptrace(PTRACE_DETACH).
func PtraceDetach(tid, sig int) error {
	var err error
	execOnPtraceThread(func() {
		_, _, err = sys.Syscall6(sys.SYS_PTRACE, sys.PTRACE_DETACH, uintptr(tid), 1, uintptr(sig), 0, 0)
	})
	if err != syscall.Errno(0) {
		return err
	}
	return nil
}

// PtraceCont executes ptrace PTRACE_CONT
func PtraceCont(tid, sig int) error {
	var err error
	execOnPtraceThread(func() { err = sys.PtraceCont(tid, sig) })
	return err
}

// PtraceSingleStep executes ptrace PTRACE_SINGLE_STEP.
func PtraceSingleStep(tid int) error {
	var err error
	execOnPtraceThread(func() { err = sys.PtraceSingleStep(tid) })
	return err
}

// PtracePokeUser executes ptrace PTRACE_POKE_USER.
func PtracePokeUser(tid int, off, addr uintptr) error {
	var err error
	execOnPtraceThread(func() {
		_, _, err = sys.Syscall6(sys.SYS_PTRACE, sys.PTRACE_POKEUSR, uintptr(tid), uintptr(off), uintptr(addr), 0, 0)
	})
	if err != syscall.Errno(0) {
		return err
	}
	return nil
}

// PtracePeekUser executes ptrace PTRACE_PEEK_USER.
func PtracePeekUser(tid int, off uintptr) (uintptr, error) {
	var val uintptr
	var err error
	execOnPtraceThread(func() {
		_, _, err = syscall.Syscall6(syscall.SYS_PTRACE, syscall.PTRACE_PEEKUSR, uintptr(tid), uintptr(off), uintptr(unsafe.Pointer(&val)), 0, 0)
	})
	if err != syscall.Errno(0) {
		return 0, err
	}
	return val, nil
}

// PtracePokeData executes ptrace PTRACE_POKE_DATA.
func PtracePokeData(tid int, addr uintptr, data []byte) (int, error) {
	var err error
	var written int
	execOnPtraceThread(func() { written, err = sys.PtracePokeData(tid, addr, data) })
	if err != nil {
		return 0, err
	}
	return written, nil
}

// PtracePeekData executes ptrace PTRACE_PEEK_DATA.
func PtracePeekData(tid int, addr uintptr, size int) ([]byte, error) {
	var err error
	data := make([]byte, size)
	execOnPtraceThread(func() { _, err = sys.PtracePeekData(tid, addr, data) })
	if err != nil {
		return nil, err
	}
	return data, nil
}

func PtraceSetOptions(pid int, options int) error {
	var err error
	execOnPtraceThread(func() { err = syscall.PtraceSetOptions(pid, syscall.PTRACE_O_TRACECLONE) })
	return err
}

// PtraceSetRegs executes PTRACE_SET_REGS.
func PtraceSetRegs(tid int, regs *sys.PtraceRegs) error {
	var err error
	execOnPtraceThread(func() { err = sys.PtraceSetRegs(tid, regs) })
	return err
}

// PtraceSetRegs executes PTRACE_GET_REGS.
func PtraceGetRegs(tid int, regs *sys.PtraceRegs) error {
	var err error
	execOnPtraceThread(func() { err = sys.PtraceGetRegs(tid, regs) })
	return err
}

func PtraceGetEventMsg(pid int) (uint, error) {
	var err error
	var cloned uint
	execOnPtraceThread(func() { cloned, err = sys.PtraceGetEventMsg(pid) })
	return cloned, err
}
