package proc

import "syscall"

type mem struct {
	id syscall.Handle
}

func newMemory(p *Process, tid int) *mem {
	return &mem{commonmem: commonmem{arch: p.arch}, id: p.os.hProcess}
}

func read(tid syscall.Handle, addr uint64, size int) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	var count uintptr
	buf := make([]byte, size)
	err := _ReadProcessMemory(tid, uintptr(addr), &buf[0], uintptr(size), &count)
	if err != nil {
		return nil, err
	}
	return buf[:count], nil
}

func write(tid syscall.Handle, addr uint64, data []byte) (int, error) {
	var count uintptr
	err := _WriteProcessMemory(tid, uintptr(addr), &data[0], uintptr(len(data)), &count)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
