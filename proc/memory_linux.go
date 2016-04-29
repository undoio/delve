package proc

func read(tid int, addr uint64, size int) ([]byte, error) {
	if size == 0 {
		return []byte{}, nil
	}
	return PtracePeekData(tid, uintptr(addr), size)
}

func write(tid int, addr uint64, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return PtracePokeData(tid, uintptr(addr), data)
}
