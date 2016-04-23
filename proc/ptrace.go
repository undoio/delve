//+build !windows
package proc

import "runtime"

var (
	ptraceChan     chan func()
	ptraceDoneChan chan struct{}
)

func onPtraceThread(fn func()) {
	ptraceChan <- fn
	<-ptraceDoneChan
}

func handlePtraceFuncs() {
	// We must ensure here that we are running on the same thread during
	// while invoking the ptrace(2) syscall. This is due to the fact that ptrace(2) expects
	// all commands after PTRACE_ATTACH to come from the same thread.
	runtime.LockOSThread()

	for fn := range ptraceChan {
		fn()
		ptraceDoneChan <- struct{}{}
	}
}

func init() {
	go handlePtraceFuncs()
}
