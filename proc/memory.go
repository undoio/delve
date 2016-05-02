package proc

const cacheEnabled = true

type MemoryReadWriter interface {
	Read(uint64, int) ([]byte, error)
	Write(uint64, []byte) (int, error)
	Swap(uint64, []byte) ([]byte, error)
}

var _ MemoryReadWriter = &memory{}

func (m *memory) Read(addr uint64, size int) ([]byte, error) {
	return read(m.id, addr, size)
}

func (m *memory) Write(addr uint64, data []byte) (int, error) {
	return write(m.id, addr, data)
}

func (m *memory) Swap(addr uint64, data []byte) ([]byte, error) {
	originalData, err := read(m.id, addr, len(data))
	if err != nil {
		return nil, err
	}
	if _, err := write(m.id, addr, data); err != nil {
		return nil, err
	}
	return originalData, nil
}

type memCache struct {
	cacheAddr uint64
	cache     []byte
	mem       MemoryReadWriter
}

func (m *memCache) contains(addr uint64, size int) bool {
	return addr >= m.cacheAddr && (addr+uint64(size)) <= (m.cacheAddr+uint64(len(m.cache)))
}

func (m *memCache) Read(addr uint64, size int) (data []byte, err error) {
	if m.contains(addr, size) {
		d := make([]byte, size)
		copy(d, m.cache[addr-m.cacheAddr:])
		return d, nil
	}

	return m.mem.Read(addr, size)
}

func (m *memCache) Write(addr uint64, data []byte) (written int, err error) {
	return m.mem.Write(addr, data)
}

func (m *memCache) Swap(addr uint64, data []byte) ([]byte, error) {
	return m.mem.Swap(addr, data)
}

func newCachedMem(mem MemoryReadWriter, addr uint64, size int) MemoryReadWriter {
	if !cacheEnabled {
		return mem
	}
	if size <= 0 {
		return mem
	}
	if cacheMem, isCache := mem.(*memCache); isCache {
		if cacheMem.contains(addr, size) {
			return mem
		} else {
			cache, err := cacheMem.mem.Read(addr, size)
			if err != nil {
				return mem
			}
			return &memCache{addr, cache, mem}
		}
	}
	cache, err := mem.Read(addr, size)
	if err != nil {
		return mem
	}
	return &memCache{addr, cache, mem}
}
