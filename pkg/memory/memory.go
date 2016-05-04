package memory

import "github.com/derekparker/delve/pkg/arch"

type ReadWriter interface {
	Read(uint64, int) ([]byte, error)
	Write(uint64, []byte) (int, error)
	Swap(uint64, []byte) ([]byte, error)
	Arch() arch.Arch
}

type Cache struct {
	cacheAddr uint64
	cache     []byte
	mem       ReadWriter
}

func NewCache(mem ReadWriter, addr uint64, size int) ReadWriter {
	if size <= 0 {
		return mem
	}
	if cacheMem, isCache := mem.(*Cache); isCache {
		if cacheMem.contains(addr, size) {
			return mem
		} else {
			cache, err := cacheMem.mem.Read(addr, size)
			if err != nil {
				return mem
			}
			return &Cache{cacheAddr: addr, cache: cache, mem: mem}
		}
	}
	cache, err := mem.Read(addr, size)
	if err != nil {
		return mem
	}
	return &Cache{cacheAddr: addr, cache: cache, mem: mem}
}

func (m *Cache) Arch() arch.Arch {
	return m.mem.Arch()
}

func (m *Cache) Read(addr uint64, size int) (data []byte, err error) {
	if m.contains(addr, size) {
		d := make([]byte, size)
		copy(d, m.cache[addr-m.cacheAddr:])
		return d, nil
	}

	return m.mem.Read(addr, size)
}

func (m *Cache) Write(addr uint64, data []byte) (written int, err error) {
	if m.contains(addr, len(data)) {
		copy(m.cache[addr-m.cacheAddr:], data)
	}
	return m.mem.Write(addr, data)
}

func (m *Cache) Swap(addr uint64, data []byte) ([]byte, error) {
	return m.mem.Swap(addr, data)
}

func (m *Cache) contains(addr uint64, size int) bool {
	return addr >= m.cacheAddr && (addr+uint64(size)) <= (m.cacheAddr+uint64(len(m.cache)))
}
