package proc

// #include "proc_darwin.h"
// #include "memory_darwin.h"
import "C"
import (
	"fmt"
	"unsafe"
)

type memory struct {
	id C.task_t
}

func newMemory(p *Process, tid int) *memory {
	return &memory{id: p.os.task}
}

func read(tid C.task_t, addr uint64, size int) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	var (
		buf    = make([]byte, size)
		vmData = unsafe.Pointer(&buf[0])
		vmAddr = C.mach_vm_address_t(addr)
		length = C.mach_msg_type_number_t(size)
	)
	ret := C.read_memory(C.task_t(tid), vmAddr, vmData, length)
	if ret < 0 {
		return nil, fmt.Errorf("memory: could not read memory, code: %d", ret)
	}
	return buf, nil
}

func write(tid C.task_t, addr uint64, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var (
		vmData = unsafe.Pointer(&data[0])
		vmAddr = C.mach_vm_address_t(addr)
		length = C.mach_msg_type_number_t(len(data))
	)
	if ret := C.write_memory(C.task_t(tid), vmAddr, vmData, length); ret < 0 {
		return 0, fmt.Errorf("memory: could not write memory, code: %d", ret)
	}
	return len(data), nil
}
