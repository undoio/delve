package proc

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	sys "golang.org/x/sys/unix"
)

// Process statuses
const (
	StatusSleeping  = 'S'
	StatusRunning   = 'R'
	StatusTraceStop = 't'
	StatusZombie    = 'Z'

	// Kernel 2.6 has TraceStop as T
	// TODO(derekparker) Since this means something different based on the
	// version of the kernel ('T' is job control stop on modern 3.x+ kernels) we
	// may want to differentiate at some point.
	StatusTraceStopT = 'T'
)

type LinuxProcess struct {
	BasicProcess
}

func (p *LinuxProcess) Continue() error {
	return p.threads.Each(continueThread)
}

func (p *LinuxProcess) Stop() error {
	return sys.Kill(p.pid, sys.SIGTRAP)
}

func (p *LinuxProcess) Kill() error {
	return sys.Kill(-p.pid, sys.SIGKILL)
}

func (p *LinuxProcess) Mourn() error {
	_, err := wait(p.pid, 0)
	return err
}

func (p *LinuxProcess) Wait() (*WaitStatus, error) {
	return wait(p.pid, 0)
}

func attach(pid int) (Process, error) {
	var err error
	onPtraceThread(func() { err = PtraceAttach(pid) })
	if err != nil {
		return nil, err
	}
	_, err = wait(pid, 0)
	if err != nil {
		return nil, err
	}
	return newproc(pid), nil
}

func launch(cmd []string) (Process, error) {
	var err error
	var pid int
	onPtraceThread(func() { pid, err = forkChild(cmd) })
	if err != nil {
		return nil, err
	}
	_, err = wait(pid, 0)
	if err != nil {
		return nil, fmt.Errorf("waiting for target execve failed: %s", err)
	}
	return newproc(pid), nil
}

func forkChild(cmd []string) (int, error) {
	proc := exec.Command(cmd[0])
	proc.Args = cmd
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.SysProcAttr = &syscall.SysProcAttr{Ptrace: true, Setpgid: true}
	if err := proc.Start(); err != nil {
		return -1, err
	}
	return proc.Process.Pid, nil
}

func newproc(pid int) *LinuxProcess {
	return &LinuxProcess{
		BasicProcess{
			pid:       pid,
			threads:   make(ThreadMap),
			statusMut: &sync.RWMutex{},
		},
	}
}

func wait(pid, options int) (*WaitStatus, error) {
	var s sys.WaitStatus

	// If we call wait4/waitpid on a thread that is the leader of its group,
	// with options == 0, while ptracing and the thread leader has exited leaving
	// zombies of its own then waitpid hangs forever this is apparently intended
	// behaviour in the linux kernel because it's just so convenient.
	// Therefore we call wait4 in a loop with WNOHANG, sleeping a while between
	// calls and exiting when either wait4 succeeds or we find out that the thread
	// has become a zombie.
	// References:
	// https://sourceware.org/bugzilla/show_bug.cgi?id=12702
	// https://sourceware.org/bugzilla/show_bug.cgi?id=10095
	// https://sourceware.org/bugzilla/attachment.cgi?id=5685
	for {
		wpid, err := sys.Wait4(pid, &s, sys.WNOHANG|sys.WALL|options, nil)
		if err != nil {
			return nil, err
		}
		if wpid != 0 || status(pid, comm(pid)) == StatusZombie {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return &WaitStatus{
		Exited: s.Exited(),
		Signal: s.Signal(),
	}, nil
}

func comm(pid int) string {
	comm, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		log.Printf("Could not read process comm name: %v\n", err)
		return ""
	}
	// removes newline character
	comm = comm[:len(comm)-1]
	return strings.Replace(string(comm), "%", "%%", -1)
}

func status(pid int, comm string) rune {
	f, err := os.Open(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return '\000'
	}
	defer f.Close()

	var (
		p     int
		state rune
	)

	// The second field of /proc/pid/stat is the name of the task in parenthesis.
	// The name of the task is the base name of the executable for this process limited to TASK_COMM_LEN characters
	// Since both parenthesis and spaces can appear inside the name of the task and no escaping happens we need to read the name of the executable first
	// See: include/linux/sched.c:315 and include/linux/sched.c:1510
	fmt.Fscanf(f, "%d ("+comm+")  %c", &p, &state)
	return state
}

// func (p *Process) setCurrentBreakpoints(trapthread *Thread) error {
// 	for _, th := range p.Threads {
// 		if th.CurrentBreakpoint == nil {
// 			err := th.SetCurrentBreakpoint()
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// // OSProcessDetails contains Li}nux specific
// // process details.
// type OSProcessDetails struct {
// 	comm string
// }

// // Launch creates and begins debugging a new process. First entry in
// // `cmd` is the program to run, and then rest are the arguments
// // to be supplied to that process.
// func Launch(cmd []string) (*Process, error) {
// 	var (
// 		proc *exec.Cmd
// 		err  error
// 	)
// 	p := New(0)
// 	p.execPtraceFunc(func() {
// 		proc = exec.Command(cmd[0])
// 		proc.Args = cmd
// 		proc.Stdout = os.Stdout
// 		proc.Stderr = os.Stderr
// 		proc.SysProcAttr = &syscall.SysProcAttr{Ptrace: true, Setpgid: true}
// 		err = proc.Start()
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	p.Pid = proc.Process.Pid
// 	_, _, err = p.wait(proc.Process.Pid, 0)
// 	if err != nil {
// 		return nil, fmt.Errorf("waiting for target execve failed: %s", err)
// 	}
// 	return initializeDebugProcess(p, proc.Path, false)
// }

// // Attach to an existing process with the given PID.
// func Attach(pid int) (*Process, error) {
// 	return initializeDebugProcess(New(pid), "", true)
// }

// // Kill kills the target process.
// func (p *Process) Kill() (err error) {
// 	if p.exited {
// 		return nil
// 	}
// 	if !p.Threads[p.Pid].Stopped() {
// 		return errors.New("process must be stopped in order to kill it")
// 	}
// 	if err = sys.Kill(-p.Pid, sys.SIGKILL); err != nil {
// 		return errors.New("could not deliver signal " + err.Error())
// 	}
// 	if _, _, err = p.wait(p.Pid, 0); err != nil {
// 		return
// 	}
// 	p.postExit()
// 	return
// }

// func (p *Process) requestManualStop() (err error) {
// 	return sys.Kill(p.Pid, sys.SIGTRAP)
// }

// // Attach to a newly created thread, and store that thread in our list of
// // known threads.
// func (p *Process) addThread(tid int, attach bool) (*Thread, error) {
// 	if thread, ok := p.Threads[tid]; ok {
// 		return thread, nil
// 	}

// 	var err error
// 	if attach {
// 		p.execPtraceFunc(func() { err = sys.PtraceAttach(tid) })
// 		if err != nil && err != sys.EPERM {
// 			// Do not return err if err == EPERM,
// 			// we may already be tracing this thread due to
// 			// PTRACE_O_TRACECLONE. We will surely blow up later
// 			// if we truly don't have permissions.
// 			return nil, fmt.Errorf("could not attach to new thread %d %s", tid, err)
// 		}
// 		pid, status, err := p.wait(tid, 0)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if status.Exited() {
// 			return nil, fmt.Errorf("thread already exited %d", pid)
// 		}
// 	}

// 	p.execPtraceFunc(func() { err = syscall.PtraceSetOptions(tid, syscall.PTRACE_O_TRACECLONE) })
// 	if err == syscall.ESRCH {
// 		if _, _, err = p.wait(tid, 0); err != nil {
// 			return nil, fmt.Errorf("error while waiting after adding thread: %d %s", tid, err)
// 		}
// 		p.execPtraceFunc(func() { err = syscall.PtraceSetOptions(tid, syscall.PTRACE_O_TRACECLONE) })
// 		if err == syscall.ESRCH {
// 			return nil, err
// 		}
// 		if err != nil {
// 			return nil, fmt.Errorf("could not set options for new traced thread %d %s", tid, err)
// 		}
// 	}

// 	p.Threads[tid] = &Thread{
// 		ID: tid,
// 		p:  p,
// 		os: new(OSSpecificDetails),
// 	}
// 	if p.CurrentThread == nil {
// 		p.SwitchThread(tid)
// 	}
// 	return p.Threads[tid], nil
// }

// func (p *Process) updateThreadList() error {
// 	tids, _ := filepath.Glob(fmt.Sprintf("/proc/%d/task/*", p.Pid))
// 	for _, tidpath := range tids {
// 		tidstr := filepath.Base(tidpath)
// 		tid, err := strconv.Atoi(tidstr)
// 		if err != nil {
// 			return err
// 		}
// 		if _, err := p.addThread(tid, tid != p.Pid); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (p *Process) findExecutable(path string) (*elf.File, error) {
// 	if path == "" {
// 		path = fmt.Sprintf("/proc/%d/exe", p.Pid)
// 	}
// 	f, err := os.OpenFile(path, 0, os.ModePerm)
// 	if err != nil {
// 		return nil, err
// 	}
// 	elfFile, err := elf.NewFile(f)
// 	if err != nil {
// 		return nil, err
// 	}
// 	p.dwarf, err = elfFile.DWARF()
// 	if err != nil {
// 		return nil, err
// 	}
// 	return elfFile, nil
// }

// func (p *Process) parseDebugFrame(exe *elf.File, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	debugFrameSec := exe.Section(".debug_frame")
// 	debugInfoSec := exe.Section(".debug_info")

// 	if debugFrameSec != nil && debugInfoSec != nil {
// 		debugFrame, err := exe.Section(".debug_frame").Data()
// 		if err != nil {
// 			fmt.Println("could not get .debug_frame section", err)
// 			os.Exit(1)
// 		}
// 		dat, err := debugInfoSec.Data()
// 		if err != nil {
// 			fmt.Println("could not get .debug_info section", err)
// 			os.Exit(1)
// 		}
// 		p.frameEntries = frame.Parse(debugFrame, frame.DwarfEndian(dat))
// 	} else {
// 		fmt.Println("could not find .debug_frame section in binary")
// 		os.Exit(1)
// 	}
// }

// func (p *Process) obtainGoSymbols(exe *elf.File, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	var (
// 		symdat  []byte
// 		pclndat []byte
// 		err     error
// 	)

// 	if sec := exe.Section(".gosymtab"); sec != nil {
// 		symdat, err = sec.Data()
// 		if err != nil {
// 			fmt.Println("could not get .gosymtab section", err)
// 			os.Exit(1)
// 		}
// 	}

// 	if sec := exe.Section(".gopclntab"); sec != nil {
// 		pclndat, err = sec.Data()
// 		if err != nil {
// 			fmt.Println("could not get .gopclntab section", err)
// 			os.Exit(1)
// 		}
// 	}

// 	pcln := gosym.NewLineTable(pclndat, exe.Section(".text").Addr)
// 	tab, err := gosym.NewTable(symdat, pcln)
// 	if err != nil {
// 		fmt.Println("could not get initialize line table", err)
// 		os.Exit(1)
// 	}

// 	p.goSymTable = tab
// }

// func (p *Process) parseDebugLineInfo(exe *elf.File, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	if sec := exe.Section(".debug_line"); sec != nil {
// 		debugLine, err := exe.Section(".debug_line").Data()
// 		if err != nil {
// 			fmt.Println("could not get .debug_line section", err)
// 			os.Exit(1)
// 		}
// 		p.lineInfo = line.Parse(debugLine)
// 	} else {
// 		fmt.Println("could not find .debug_line section in binary")
// 		os.Exit(1)
// 	}
// }

// func (p *Process) wait(options int) (*Status, error) {
// 	var s sys.WaitStatus

// 	// If we call wait4/waitpid on a thread that is the leader of its group,
// 	// with options == 0, while ptracing and the thread leader has exited leaving
// 	// zombies of its own then waitpid hangs forever this is apparently intended
// 	// behaviour in the linux kernel because it's just so convenient.
// 	// Therefore we call wait4 in a loop with WNOHANG, sleeping a while between
// 	// calls and exiting when either wait4 succeeds or we find out that the thread
// 	// has become a zombie.
// 	// References:
// 	// https://sourceware.org/bugzilla/show_bug.cgi?id=12702
// 	// https://sourceware.org/bugzilla/show_bug.cgi?id=10095
// 	// https://sourceware.org/bugzilla/attachment.cgi?id=5685
// 	for {
// 		wpid, err := sys.Wait4(pid, &s, sys.WNOHANG|sys.WALL|options, nil)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if wpid != 0 {
// 			break
// 		}
// 		if status(dbp.Pid, p.os.comm) == StatusZombie {
// 			return dbp.Pid, nil, nil
// 		}
// 		time.Sleep(200 * time.Millisecond)
// 	}

// 	th, ok := p.Threads[wpid]
// 	if !ok {
// 		return p.wait(options)
// 	}
// 	th.Status = (*WaitStatus)(status)
// 	if status.Exited() {
// 		if wpid == p.Pid {
// 			return nil, p.Mourn()
// 		}
// 		delete(p.Threads, wpid)
// 	}
// 	if status.StopSignal() == sys.SIGTRAP {
// 		if status.TrapCause() == sys.PTRACE_EVENT_CLONE {
// 			// A traced thread has cloned a new thread, grab the pid and
// 			// add it to our list of traced threads.
// 			var cloned uint
// 			p.execPtraceFunc(func() { cloned, err = sys.PtraceGetEventMsg(wpid) })
// 			if err != nil {
// 				return nil, fmt.Errorf("could not get event message: %s", err)
// 			}
// 			nth, err := p.addThread(int(cloned), false)
// 			if err != nil {
// 				if err == sys.ESRCH {
// 					// thread died while we were adding it
// 					delete(p.Threads, nth.ID)
// 					return p.wait(options)
// 				}
// 				return nil, err
// 			} else {
// 				if err = nth.Continue(); err != nil {
// 					return nil, fmt.Errorf("could not continue new thread %d %s", nth.ID, err)
// 				}
// 				if err = th.Continue(); err != nil {
// 					return nil, fmt.Errorf("could not continue existing thread %d %s", th.ID, err)
// 				}
// 			}
// 			return p.wait(options)
// 		}
// 		if p.halt {
// 			p.halt = false
// 		}
// 		th.running = false
// 		return th, nil
// 	}
// 	// TODO(derekparker) alert user about unexpected signals here.
// 	if err := th.resumeWithSig(int(status.StopSignal())); err != nil {
// 		if err == sys.ESRCH {
// 			return nil, ProcessExitedError{Pid: p.Pid}
// 		}
// 		return nil, err
// 	}
// }

// func (p *Process) loadProcessInformation(wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	comm, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/comm", p.Pid))
// 	if err != nil {
// 		fmt.Printf("Could not read process comm name: %v\n", err)
// 		os.Exit(1)
// 	}
// 	// removes newline character
// 	comm = comm[:len(comm)-1]
// 	p.os.comm = strings.Replace(string(comm), "%", "%%", -1)
// }

// func (p *Process) exitGuard(err error) error {
// 	if err != sys.ESRCH {
// 		return err
// 	}
// 	if status(p.Pid, p.os.comm) == StatusZombie {
// 		_, err := p.trapWait(-1)
// 		return err
// 	}

// 	return err
// }

// func (p *Process) resume() error {
// 	// all threads stopped over a breakpoint are made to step over it
// 	for _, thread := range p.Threads {
// 		if thread.CurrentBreakpoint != nil {
// 			if err := thread.StepInstruction(); err != nil {
// 				return err
// 			}
// 			thread.CurrentBreakpoint = nil
// 		}
// 	}
// 	// everything is resumed
// 	for _, thread := range p.Threads {
// 		if err := thread.resume(); err != nil && err != sys.ESRCH {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func killProcess(pid int) error {
// 	return sys.Kill(pid, sys.SIGINT)
// }
