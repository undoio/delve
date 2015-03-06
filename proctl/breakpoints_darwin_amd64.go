package proctl

// #include "breakpoints_darwin_amd64.h"
import "C"
import "fmt"

func getControlRegister(port int) (uintptr, error) {
	dr7, err := C.get_control_register(C.thread_act_t(port))
	if err != nil {
		return 0, fmt.Errorf("could not get control register")
	}
	return uintptr(dr7), nil
}

func setControlRegister(port int, dr7 uintptr) error {
	kret := C.set_control_register(C.thread_act_t(port), C.uint64_t(dr7))
	if kret != C.KERN_SUCCESS {
		return fmt.Errorf("could not set control register")
	}
	return nil
}

func setDebugRegister(port int, reg int, addr uintptr) error {
	kret := C.set_debug_register(C.thread_act_t(port), C.int(reg), C.uint64_t(addr))
	if kret != C.KERN_SUCCESS {
		return fmt.Errorf("could not set debug register")
	}
	return nil
}
