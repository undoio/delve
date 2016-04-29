package proc

import "github.com/derekparker/delve/proc/internal/mssys"

func read(tid int, addr uint64, size int) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	var count uintptr
	buf := make([]byte, size)
	err := mssys.ReadProcessMemory(t.dbp.os.hProcess, addr, &buf[0], uintptr(size), &count)
	if err != nil {
		return nil, err
	}
	return buf[:count], nil
}

func write(tid int, addr uint64, data []byte) (int, error) {
	var count uintptr
	err := mssys.WriteProcessMemory(t.dbp.os.hProcess, addr, &data[0], uintptr(len(data)), &count)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
