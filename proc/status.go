package proc

import "syscall"

type ProcessStatus struct {
	exited     bool
	exitStatus int
	signal     syscall.Signal
}

func (ps *ProcessStatus) Exited() bool {
	return ps.exited
}

func (ps *ProcessStatus) ExitStatus() int {
	return ps.exitStatus
}

func (ps *ProcessStatus) Signal() syscall.Signal {
	return ps.signal
}
