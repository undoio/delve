package proc

import (
	"github.com/derekparker/delve/pkg/arch"
	"github.com/derekparker/delve/pkg/memory"
)

type commonmem struct {
	arch arch.Arch
}

// Assert mem conforms to interface.
var _ memory.ReadWriter = &mem{}

func (m *mem) Read(addr uint64, size int) ([]byte, error) {
	return read(m.id, addr, size)
}

func (m *mem) Write(addr uint64, data []byte) (int, error) {
	return write(m.id, addr, data)
}

func (m *mem) Swap(addr uint64, data []byte) ([]byte, error) {
	originalData, err := read(m.id, addr, len(data))
	if err != nil {
		return nil, err
	}
	if _, err := write(m.id, addr, data); err != nil {
		return nil, err
	}
	return originalData, nil
}

func (m *mem) Arch() arch.Arch {
	return m.arch
}
