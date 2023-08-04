//go:build darwin || (windows && arm64)
// +build darwin windows,arm64

package native

import (
	"github.com/undoio/delve/pkg/elfwriter"
	"github.com/undoio/delve/pkg/proc"
)

func (p *nativeProcess) MemoryMap() ([]proc.MemoryMapEntry, error) {
	return nil, proc.ErrMemoryMapNotSupported
}

func (p *nativeProcess) DumpProcessNotes(notes []elfwriter.Note, threadDone func()) (threadsDone bool, notesout []elfwriter.Note, err error) {
	return false, notes, nil
}
