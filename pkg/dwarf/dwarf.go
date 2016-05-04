package dwarf

import (
	"debug/gosym"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/debug/dwarf"

	"github.com/derekparker/delve/pkg/dwarf/frame"
	"github.com/derekparker/delve/pkg/dwarf/line"
	"github.com/derekparker/delve/pkg/dwarf/reader"
)

// Do not call this function directly it isn't able to deal correctly with package paths
func (d *Dwarf) findType(name string) (dwarf.Type, error) {
	off, found := d.Types[name]
	if !found {
		return nil, reader.TypeNotFoundErr
	}
	return d.Type(off)
}

func (d *Dwarf) FindTypeExpr(expr ast.Expr) (dwarf.Type, error) {
	if lit, islit := expr.(*ast.BasicLit); islit && lit.Kind == token.STRING {
		// Allow users to specify type names verbatim as quoted
		// string. Useful as a catch-all workaround for cases where we don't
		// parse/serialize types correctly or can not resolve package paths.
		typn, _ := strconv.Unquote(lit.Value)
		return d.findType(typn)
	}
	expandPackagesInType(d.Packages, expr)
	if snode, ok := expr.(*ast.StarExpr); ok {
		// Pointer types only appear in the dwarf informations when
		// a pointer to the type is used in the target program, here
		// we create a pointer type on the fly so that the user can
		// specify a pointer to any variable used in the target program
		ptyp, err := d.FindTypeExpr(snode.X)
		if err != nil {
			return nil, err
		}
		return PointerTo(ptyp), nil
	}
	return d.findType(exprToString(expr))
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

func expandPackagesInType(packages map[string]string, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.ArrayType:
		expandPackagesInType(packages, e.Elt)
	case *ast.ChanType:
		expandPackagesInType(packages, e.Value)
	case *ast.FuncType:
		for i := range e.Params.List {
			expandPackagesInType(packages, e.Params.List[i].Type)
		}
		if e.Results != nil {
			for i := range e.Results.List {
				expandPackagesInType(packages, e.Results.List[i].Type)
			}
		}
	case *ast.MapType:
		expandPackagesInType(packages, e.Key)
		expandPackagesInType(packages, e.Value)
	case *ast.ParenExpr:
		expandPackagesInType(packages, e.X)
	case *ast.SelectorExpr:
		switch x := e.X.(type) {
		case *ast.Ident:
			if path, ok := packages[x.Name]; ok {
				x.Name = path
			}
		default:
			expandPackagesInType(packages, e.X)
		}
	case *ast.StarExpr:
		expandPackagesInType(packages, e.X)
	default:
		// nothing to do
	}
}

type Dwarf struct {
	Frame    frame.FrameDescriptionEntries
	Line     line.DebugLines
	Types    map[string]dwarf.Offset
	Packages map[string]string

	// We keep this here so we have all debug info
	// in a single place. Not technically dwarf.
	symboltab *gosym.Table

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
	syms, err := parseGoSymbols(exe)
	if err != nil {
		return nil, err
	}
	rdr := reader.New(d)
	dw := &Dwarf{
		Frame:     frame,
		Line:      line,
		data:      d,
		Types:     loadTypes(rdr),
		Packages:  loadPackages(rdr),
		symboltab: syms,
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

func (d *Dwarf) PCToLine(pc uint64) (string, int, *gosym.Func) {
	return d.symboltab.PCToLine(pc)
}

func (d *Dwarf) PCToFunc(pc uint64) *gosym.Func {
	return d.symboltab.PCToFunc(pc)
}

func (d *Dwarf) LineToPC(file string, line int) (uint64, *gosym.Func, error) {
	return d.symboltab.LineToPC(file, line)
}

func (d *Dwarf) Funcs() []gosym.Func {
	return d.symboltab.Funcs
}

func (d *Dwarf) LookupFunc(name string) *gosym.Func {
	return d.symboltab.LookupFunc(name)
}

func (d *Dwarf) Files() map[string]*gosym.Obj {
	return d.symboltab.Files
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
	rdr.Seek(0)
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
