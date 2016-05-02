package dwarf

import (
	"reflect"
	"strings"
	"unsafe"

	"golang.org/x/debug/dwarf"

	"github.com/derekparker/delve/pkg/dwarf/frame"
	"github.com/derekparker/delve/pkg/dwarf/line"
	"github.com/derekparker/delve/pkg/dwarf/reader"
)

type Dwarf struct {
	Frame    frame.FrameDescriptionEntries
	Line     line.DebugLines
	Types    map[string]dwarf.Offset
	Packages map[string]string

	data *dwarf.Data
}

func Parse(path string) (*Dwarf, error) {
	exe, err := newExecutable(path)
	if err != nil {
		return nil, err
	}
	d, err := parseDwarf(exe)
	if err != nil {
		return nil, err
	}
	line, err := parseLine(exe)
	if err != nil {
		return nil, err
	}
	frame, err := parseFrame(exe)
	if err != nil {
		return nil, err
	}
	rdr := reader.New(d)
	dw := &Dwarf{
		Frame:    frame,
		Line:     line,
		data:     d,
		Types:    loadTypes(rdr),
		Packages: loadPackages(rdr),
	}
	return dw, nil
}

func (d *Dwarf) Reader() *reader.Reader {
	return reader.New(d.data)
}

func (d *Dwarf) Type(off dwarf.Offset) (dwarf.Type, error) {
	return d.data.Type(off)
}

func (d *Dwarf) TypeNamed(name string) (dwarf.Type, error) {
	off, found := d.Types[name]
	if !found {
		return nil, reader.TypeNotFoundErr
	}
	return d.data.Type(off)
}

func loadTypes(rdr *reader.Reader) map[string]dwarf.Offset {
	types := make(map[string]dwarf.Offset)
	rdr.Seek(0)
	for entry, err := rdr.NextType(); entry != nil; entry, err = rdr.NextType() {
		if err != nil {
			break
		}
		name, ok := entry.Val(dwarf.AttrName).(string)
		if !ok {
			continue
		}
		if _, exists := types[name]; !exists {
			types[name] = entry.Offset
		}
	}
	return types
}

func loadPackages(rdr *reader.Reader) map[string]string {
	packages := map[string]string{}
	for entry, err := rdr.Next(); entry != nil; entry, err = rdr.Next() {
		if err != nil {
			// TODO(derekparker) do not panic here
			panic(err)
		}

		if entry.Tag != dwarf.TagTypedef && entry.Tag != dwarf.TagBaseType && entry.Tag != dwarf.TagClassType && entry.Tag != dwarf.TagStructType {
			continue
		}

		typename, ok := entry.Val(dwarf.AttrName).(string)
		if !ok || complexType(typename) {
			continue
		}

		dot := strings.LastIndex(typename, ".")
		if dot < 0 {
			continue
		}
		path := typename[:dot]
		slash := strings.LastIndex(path, "/")
		if slash < 0 || slash+1 >= len(path) {
			continue
		}
		name := path[slash+1:]
		packages[name] = path
	}
	return packages
}

func PointerTo(typ dwarf.Type) dwarf.Type {
	return &dwarf.PtrType{dwarf.CommonType{int64(unsafe.Sizeof(uintptr(1))), "", reflect.Ptr, 0}, typ}
}

func complexType(typename string) bool {
	for _, ch := range typename {
		switch ch {
		case '*', '[', '<', '{', '(', ' ':
			return true
		}
	}
	return false
}
