package dwarf

import (
	"errors"
	"fmt"

	"github.com/derekparker/delve/pkg/dwarf/frame"
	"github.com/derekparker/delve/pkg/dwarf/line"
	"golang.org/x/debug/macho"
)

func newExecutable(path string) (*macho.File, error) {
	return macho.Open(path)
}

func parseFrame(exe *macho.File) (frame.FrameDescriptionEntries, error) {
	debugFrameSec := exe.Section("__debug_frame")
	debugInfoSec := exe.Section("__debug_info")

	if debugFrameSec != nil && debugInfoSec != nil {
		debugFrame, err := exe.Section("__debug_frame").Data()
		if err != nil {
			return nil, fmt.Errorf("dwarf: could not get __debug_frame section: %v", err)
		}
		dat, err := debugInfoSec.Data()
		if err != nil {
			return nil, fmt.Errorf("dwarf: could not get .debug_info section: %v", err)
		}
		return frame.Parse(debugFrame, frame.DwarfEndian(dat)), nil
	}
	return nil, errors.New("dwarf: could not find __debug_frame section in binary")
}

func parseLine(exe *macho.File) (line.DebugLines, error) {
	if sec := exe.Section("__debug_line"); sec != nil {
		debugLine, err := exe.Section("__debug_line").Data()
		if err != nil {
			return nil, fmt.Errorf("dwarf: could not get __debug_line section: %v", err)
		}
		return line.Parse(debugLine), nil
	}
	return nil, errors.New("dwarf: could not find __debug_line section in binary")
}
