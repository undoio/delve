package dwarf

import (
	"debug/gosym"
	"debug/pe"
	"errors"
	"fmt"

	"golang.org/x/debug/dwarf"

	"github.com/derekparker/delve/pkg/dwarf/frame"
	"github.com/derekparker/delve/pkg/dwarf/line"
)

func newExecutable(path string) (*pe.File, error) {
	return pe.Open(path)
}

func parseFrame(exe *pe.File) (frame.FrameDescriptionEntries, error) {
	debugFrameSec := exe.Section(".debug_frame")
	debugInfoSec := exe.Section(".debug_info")

	if debugFrameSec != nil && debugInfoSec != nil {
		debugFrame, err := debugFrameSec.Data()
		if err != nil && uint32(len(debugFrame)) < debugFrameSec.Size {
			return nil, fmt.Errorf("dwarf: could not get .debug_frame section: %v", err)
		}
		if 0 < debugFrameSec.VirtualSize && debugFrameSec.VirtualSize < debugFrameSec.Size {
			debugFrame = debugFrame[:debugFrameSec.VirtualSize]
		}
		dat, err := debugInfoSec.Data()
		if err != nil {
			return nil, fmt.Errorf("dwarf: could not get .debug_info section: %v", err)
		}
		return frame.Parse(debugFrame, frame.DwarfEndian(dat)), nil
	}
	return nil, errors.New("dwarf: could not find .debug_frame section in binary")
}

func parseLine(exe *pe.File) (line.DebugLines, error) {
	if sec := exe.Section(".debug_line"); sec != nil {
		debugLine, err := sec.Data()
		if err != nil && uint32(len(debugLine)) < sec.Size {
			return nil, fmt.Errorf("dwarf: could not get .debug_line section: %v", err)
		}
		if 0 < sec.VirtualSize && sec.VirtualSize < sec.Size {
			debugLine = debugLine[:sec.VirtualSize]
		}
		return line.Parse(debugLine), nil
	}
	return nil, errors.New("dwarf: could not find .debug_line section in binary")
}

// Adapted from src/debug/pe/file.go: pe.(*File).DWARF()
func parseDwarf(f *pe.File) (*dwarf.Data, error) {
	// There are many other DWARF sections, but these
	// are the ones the debug/dwarf package uses.
	// Don't bother loading others.
	var names = [...]string{"abbrev", "info", "line", "str"}
	var dat [len(names)][]byte
	for i, name := range names {
		name = ".debug_" + name
		s := f.Section(name)
		if s == nil {
			continue
		}
		b, err := s.Data()
		if err != nil && uint32(len(b)) < s.Size {
			return nil, err
		}
		if 0 < s.VirtualSize && s.VirtualSize < s.Size {
			b = b[:s.VirtualSize]
		}
		dat[i] = b
	}

	abbrev, info, line, str := dat[0], dat[1], dat[2], dat[3]
	return dwarf.New(abbrev, nil, nil, info, line, nil, nil, str)
}

func parseGoSymbols(exe *pe.File) (*gosym.Table, error) {
	_, symdat, pclndat, err := pcln(exe)
	if err != nil {
		return nil, fmt.Errorf("dwarf: could not get Go symbols: %v", err)
	}

	pcln := gosym.NewLineTable(pclndat, uint64(exe.Section(".text").Offset))
	tab, err := gosym.NewTable(symdat, pcln)
	if err != nil {
		return nil, fmt.Errorf("dwarf: could not get initialize line table: %v", err)
	}

	return tab, nil
}
