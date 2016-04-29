package dwarf

import (
	"errors"
	"fmt"

	"debug/pe"

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
		return line.Parse(debugLine)
	}
	return nil, errors.New("dwarf: could not find .debug_line section in binary")
}
