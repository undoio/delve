package proctl

/*
#include <stddef.h>
#include <sys/types.h>
#include <sys/user.h>
#include <sys/debugreg.h>

// Exposes C macro `offsetof` which is needed for getting
// the offset of the debug register we want, and passing
// that offset to PTRACE_POKE_USER.
int offset(int reg) {
	return offsetof(struct user, u_debugreg[reg]);
}
*/
import "C"

func getControlRegister(tid int) (uintptr, error) {
	dr7off := uintptr(C.offset(C.DR_CONTROL))
	return PtracePeekUser(tid, dr7off)
}

func setControlRegister(tid int, dr7 uintptr) error {
	dr7off := uintptr(C.offset(C.DR_CONTROL))
	return PtracePokeUser(tid, dr7off, uintptr(addr))
}

func setDebugRegister(tid int, reg int, addr uintptr) error {
	drxoff := uintptr(C.offset(C.int(reg)))
	return PtracePokeUser(tid, drxoff, addr)
}
