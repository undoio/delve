package proc

import (
	"errors"
	"sync"
	"syscall"
)

// Process represents the necessary low-level methods that must
// be used to manipulate an operating system process.
//
// Methods are provided to control execution of the process, e.g.
// resume, stop, kill, wait. Everymethod applies to all threads
// in the process.
type Process interface {
	Pid() int

	Continue() error
	Wait() (*WaitStatus, error)
	Stop() error
	Kill() error

	Exited() bool

	Threads() ThreadMap
}

// WaitStatus is the status we get back
// after waiting on the process.
type WaitStatus struct {
	Exited bool
	Signal syscall.Signal
}

// Process represents all of the information the debugger
// is holding onto regarding the process we are debugging.
type BasicProcess struct {
	pid     int       // Process Pid
	threads ThreadMap // Threads in the process

	statusMut *sync.RWMutex
	status    *WaitStatus

	// // Breakpoint table, holds information on breakpoints.
	// // Maps instruction address to Breakpoint struct.
	// Breakpoints map[uint64]*Breakpoint

	// // Active thread
	// CurrentThread *Thread

	// // Goroutine that will be used by default to set breakpoint, eval variables, etc...
	// // Normally SelectedGoroutine is CurrentThread.GetG, it will not be only if SwitchGoroutine is called with a goroutine that isn't attached to a thread
	// SelectedGoroutine *G

	// // Maps package names to package paths, needed to lookup types inside DWARF info
	// packageMap map[string]string

	// allGCache               []*G
	// dwarf                   *dwarf.Data
	// goSymTable              *gosym.Table
	// frameEntries            frame.FrameDescriptionEntries
	// lineInfo                line.DebugLines
	// os                      *OSProcessDetails
	// arch                    Arch
	// breakpointIDCounter     int
	// tempBreakpointIDCounter int
	// firstStart              bool
	// halt                    bool
	// exited                  bool
	// ptraceChan              chan func()
	// ptraceDoneChan          chan interface{}
	// types                   map[string]dwarf.Offset
}

func Attach(pid int) (Process, error) {
	return attach(pid)
}

func Launch(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		return nil, errors.New("no arguments passed to launch")
	}
	return launch(cmd)
}

func (p *BasicProcess) Pid() int {
	return p.pid
}

func (p *BasicProcess) Threads() ThreadMap {
	return p.threads
}

// Exited returns whether the process has exited.
func (p *BasicProcess) Exited() bool {
	p.statusMut.RLock()
	defer func() { p.statusMut.RUnlock() }()
	return p.status.Exited
}

// // ProcessExitedError indicates that the process has exited and contains both
// // process id and exit status.
// type ProcessExitedError struct {
// 	Pid    int
// 	Status int
// }

// func (pe ProcessExitedError) Error() string {
// 	return fmt.Sprintf("Process %d has exited with status %d", pe.Pid, pe.Status)
// }

// // Detach from the process being debugged, optionally killing it.
// func (p *Process) Detach(kill bool) (err error) {
// 	if p.Running() {
// 		if err = p.Halt(); err != nil {
// 			return
// 		}
// 	}
// 	if !kill {
// 		// Clean up any breakpoints we've set.
// 		for _, bp := range p.Breakpoints {
// 			if bp != nil {
// 				_, err := p.ClearBreakpoint(bp.Addr)
// 				if err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}
// 	p.execPtraceFunc(func() {
// 		err = PtraceDetach(p.Pid, 0)
// 		if err != nil {
// 			return
// 		}
// 		if kill {
// 			err = killProcess(p.Pid)
// 		}
// 	})
// 	return
// }

// // Exited returns whether the debugged
// // process has exited.
// func (p *Process) Exited() bool {
// 	return p.exited
// }

// // Running returns whether the debugged
// // process is currently executing.
// func (p *Process) Running() bool {
// 	for _, th := range p.Threads {
// 		if th.running {
// 			return true
// 		}
// 	}
// 	return false
// }

// // LoadInformation finds the executable and then uses it
// // to parse the following information:
// // * Dwarf .debug_frame section
// // * Dwarf .debug_line section
// // * Go symbol table.
// func (p *Process) LoadInformation(path string) error {
// 	var wg sync.WaitGroup

// 	exe, err := p.findExecutable(path)
// 	if err != nil {
// 		return err
// 	}

// 	wg.Add(5)
// 	go p.loadProcessInformation(&wg)
// 	go p.parseDebugFrame(exe, &wg)
// 	go p.obtainGoSymbols(exe, &wg)
// 	go p.parseDebugLineInfo(exe, &wg)
// 	go p.loadTypeMap(&wg)
// 	wg.Wait()

// 	return nil
// }

// // FindFileLocation returns the PC for a given file:line.
// // Assumes that `file` is normailzed to lower case and '/' on Windows.
// func (p *Process) FindFileLocation(fileName string, lineno int) (uint64, error) {
// 	pc, fn, err := p.goSymTable.LineToPC(fileName, lineno)
// 	if err != nil {
// 		return 0, err
// 	}
// 	if fn.Entry == pc {
// 		pc, _ = p.FirstPCAfterPrologue(fn, true)
// 	}
// 	return pc, nil
// }

// // FindFunctionLocation finds address of a function's line
// // If firstLine == true is passed FindFunctionLocation will attempt to find the first line of the function
// // If lineOffset is passed FindFunctionLocation will return the address of that line
// // Pass lineOffset == 0 and firstLine == false if you want the address for the function's entry point
// // Note that setting breakpoints at that address will cause surprising behavior:
// // https://github.com/derekparker/delve/issues/170
// func (p *Process) FindFunctionLocation(funcName string, firstLine bool, lineOffset int) (uint64, error) {
// 	origfn := p.goSymTable.LookupFunc(funcName)
// 	if origfn == nil {
// 		return 0, fmt.Errorf("Could not find function %s\n", funcName)
// 	}

// 	if firstLine {
// 		return p.FirstPCAfterPrologue(origfn, false)
// 	} else if lineOffset > 0 {
// 		filename, lineno, _ := p.goSymTable.PCToLine(origfn.Entry)
// 		breakAddr, _, err := p.goSymTable.LineToPC(filename, lineno+lineOffset)
// 		return breakAddr, err
// 	}

// 	return origfn.Entry, nil
// }

// // CurrentLocation returns the location of the current thread.
// func (p *Process) CurrentLocation() (*Location, error) {
// 	return p.CurrentThread.Location()
// }

// // RequestManualStop sets the `halt` flag and
// // sends SIGSTOP to all threads.
// func (p *Process) RequestManualStop() error {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	p.halt = true
// 	return p.requestManualStop()
// }

// // SetBreakpoint sets a breakpoint at addr, and stores it in the process wide
// // break point table. Setting a break point must be thread specific due to
// // ptrace actions needing the thread to be in a signal-delivery-stop.
// func (p *Process) SetBreakpoint(addr uint64) (*Breakpoint, error) {
// 	if p.exited {
// 		return nil, &ProcessExitedError{}
// 	}
// 	return p.setBreakpoint(p.CurrentThread.ID, addr, false)
// }

// // SetTempBreakpoint sets a temp breakpoint. Used during 'next' operations.
// func (p *Process) SetTempBreakpoint(addr uint64) (*Breakpoint, error) {
// 	return p.setBreakpoint(p.CurrentThread.ID, addr, true)
// }

// // ClearBreakpoint clears the breakpoint at addr.
// func (p *Process) ClearBreakpoint(addr uint64) (*Breakpoint, error) {
// 	if p.exited {
// 		return nil, &ProcessExitedError{}
// 	}
// 	bp, ok := p.FindBreakpoint(addr)
// 	if !ok {
// 		return nil, NoBreakpointError{addr: addr}
// 	}

// 	if _, err := bp.Clear(p.CurrentThread); err != nil {
// 		return nil, err
// 	}

// 	delete(p.Breakpoints, addr)

// 	return bp, nil
// }

// // Status returns the status of the current main thread context.
// func (p *Process) Status() *WaitStatus {
// 	return p.CurrentThread.Status
// }

// // Next continues execution until the next source line.
// func (p *Process) Next() (err error) {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	for i := range p.Breakpoints {
// 		if p.Breakpoints[i].Temp {
// 			return fmt.Errorf("next while nexting")
// 		}
// 	}

// 	// Get the goroutine for the current thread. We will
// 	// use it later in order to ensure we are on the same
// 	// goroutine.
// 	g, err := p.CurrentThread.GetG()
// 	if err != nil {
// 		return err
// 	}

// 	// Set breakpoints for any goroutine that is currently
// 	// blocked trying to read from a channel. This is so that
// 	// if control flow switches to that goroutine, we end up
// 	// somewhere useful instead of in runtime code.
// 	if _, err = p.setChanRecvBreakpoints(); err != nil {
// 		return
// 	}

// 	var goroutineExiting bool
// 	if err = p.CurrentThread.setNextBreakpoints(); err != nil {
// 		switch t := err.(type) {
// 		case ThreadBlockedError, NoReturnAddr: // Noop
// 		case GoroutineExitingError:
// 			goroutineExiting = t.goid == g.ID
// 		default:
// 			p.clearTempBreakpoints()
// 			return
// 		}
// 	}

// 	if !goroutineExiting {
// 		for i := range p.Breakpoints {
// 			if p.Breakpoints[i].Temp {
// 				p.Breakpoints[i].Cond = &ast.BinaryExpr{
// 					Op: token.EQL,
// 					X: &ast.SelectorExpr{
// 						X: &ast.SelectorExpr{
// 							X:   &ast.Ident{Name: "runtime"},
// 							Sel: &ast.Ident{Name: "curg"},
// 						},
// 						Sel: &ast.Ident{Name: "goid"},
// 					},
// 					Y: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(g.ID)},
// 				}
// 			}
// 		}
// 	}

// 	return p.Continue()
// }

// func (p *Process) setChanRecvBreakpoints() (int, error) {
// 	var count int
// 	allg, err := p.GoroutinesInfo()
// 	if err != nil {
// 		return 0, err
// 	}

// 	for _, g := range allg {
// 		if g.ChanRecvBlocked() {
// 			ret, err := g.chanRecvReturnAddr(p)
// 			if err != nil {
// 				if _, ok := err.(NullAddrError); ok {
// 					continue
// 				}
// 				return 0, err
// 			}
// 			if _, err = p.SetTempBreakpoint(ret); err != nil {
// 				if _, ok := err.(BreakpointExistsError); ok {
// 					// Ignore duplicate breakpoints in case if multiple
// 					// goroutines wait on the same channel
// 					continue
// 				}
// 				return 0, err
// 			}
// 			count++
// 		}
// 	}
// 	return count, nil
// }

// // Continue continues execution of the debugged process.
// func (p *Process) Continue() error {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	// Clear the allG cache
// 	p.allGCache = nil
// 	return p.resume()
// }

// type Status struct {
// 	Exited     bool
// 	Signal     syscall.Signal
// 	StopThread *Thread
// }

// // Wait will wait until the child stops from either
// // a signal or because it has died.
// func (p *Process) Wait() (*Status, error) {
// 	return p.wait(0)
// }

// func (p *Process) conditionErrors() error {
// 	var condErr error
// 	for _, th := range p.Threads {
// 		if th.CurrentBreakpoint != nil && th.BreakpointConditionError != nil {
// 			if condErr == nil {
// 				condErr = th.BreakpointConditionError
// 			} else {
// 				return fmt.Errorf("multiple errors evaluating conditions")
// 			}
// 		}
// 	}
// 	return condErr
// }

// // pick a new p.CurrentThread, with the following priority:
// // 	- a thread with onTriggeredTempBreakpoint() == true
// // 	- a thread with onTriggeredBreakpoint() == true (prioritizing trapthread)
// // 	- trapthread
// func (p *Process) pickCurrentThread(trapthread *Thread) error {
// 	for _, th := range p.Threads {
// 		if th.onTriggeredTempBreakpoint() {
// 			return p.SwitchThread(th.ID)
// 		}
// 	}
// 	if trapthread.onTriggeredBreakpoint() {
// 		return p.SwitchThread(trapthread.ID)
// 	}
// 	for _, th := range p.Threads {
// 		if th.onTriggeredBreakpoint() {
// 			return p.SwitchThread(th.ID)
// 		}
// 	}
// 	return p.SwitchThread(trapthread.ID)
// }

// // Step will continue until another source line is reached.
// // Will step into functions.
// // func (p *Process) Step() (err error) {
// // 	fn := func() error {
// // 		var nloc *Location
// // 		th := p.CurrentThread
// // 		loc, err := th.Location()
// // 		if err != nil {
// // 			return err
// // 		}
// // 		for {
// // 			pc, err := p.CurrentThread.PC()
// // 			if err != nil {
// // 				return err
// // 			}
// // 			text, err := p.CurrentThread.Disassemble(pc, pc+maxInstructionLength, true)
// // 			if err == nil && len(text) > 0 && text[0].IsCall() && text[0].DestLoc != nil && text[0].DestLoc.Fn != nil {
// // 				return p.StepInto(text[0].DestLoc.Fn)
// // 			}

// // 			err = p.CurrentThread.StepInstruction()
// // 			if err != nil {
// // 				return err
// // 			}
// // 			nloc, err = th.Location()
// // 			if err != nil {
// // 				return err
// // 			}
// // 			if nloc.File != loc.File {
// // 				return nil
// // 			}
// // 			if nloc.File == loc.File && nloc.Line != loc.Line {
// // 				return nil
// // 			}
// // 		}
// // 	}
// // 	return p.run(fn)
// // }

// // StepInto sets a temp breakpoint after the prologue of fn and calls Continue
// func (p *Process) StepInto(fn *gosym.Func) error {
// 	pc, _ := p.FirstPCAfterPrologue(fn, false)
// 	if _, err := p.SetTempBreakpoint(pc); err != nil {
// 		return err
// 	}
// 	return p.Continue()
// }

// // StepInstruction will continue the current thread for exactly
// // one instruction. This method affects only the thread
// // asssociated with the selected goroutine. All other
// // threads will remain stopped.
// func (p *Process) StepInstruction() (err error) {
// 	if p.SelectedGoroutine == nil {
// 		return errors.New("cannot single step: no selected goroutine")
// 	}
// 	if p.SelectedGoroutine.thread == nil {
// 		return fmt.Errorf("cannot single step: no thread associated with goroutine %d", p.SelectedGoroutine.ID)
// 	}
// 	return p.SelectedGoroutine.thread.StepInstruction()
// }

// // SwitchThread changes from current thread to the thread specified by `tid`.
// func (p *Process) SwitchThread(tid int) error {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	if th, ok := p.Threads[tid]; ok {
// 		p.CurrentThread = th
// 		p.SelectedGoroutine, _ = p.CurrentThread.GetG()
// 		return nil
// 	}
// 	return fmt.Errorf("thread %d does not exist", tid)
// }

// // SwitchGoroutine changes from current thread to the thread
// // running the specified goroutine.
// func (p *Process) SwitchGoroutine(gid int) error {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	g, err := p.FindGoroutine(gid)
// 	if err != nil {
// 		return err
// 	}
// 	if g == nil {
// 		// user specified -1 and SelectedGoroutine is nil
// 		return nil
// 	}
// 	if g.thread != nil {
// 		return p.SwitchThread(g.thread.ID)
// 	}
// 	p.SelectedGoroutine = g
// 	return nil
// }

// // GoroutinesInfo returns an array of G structures representing the information
// // Delve cares about from the internal runtime G structure.
// func (p *Process) GoroutinesInfo() ([]*G, error) {
// 	if p.exited {
// 		return nil, &ProcessExitedError{}
// 	}
// 	if p.allGCache != nil {
// 		return p.allGCache, nil
// 	}

// 	var (
// 		threadg = map[int]*Thread{}
// 		allg    []*G
// 		rdr     = p.DwarfReader()
// 	)

// 	for i := range p.Threads {
// 		if p.Threads[i].blocked() {
// 			continue
// 		}
// 		g, _ := p.Threads[i].GetG()
// 		if g != nil {
// 			threadg[g.ID] = p.Threads[i]
// 		}
// 	}

// 	addr, err := rdr.AddrFor("runtime.allglen")
// 	if err != nil {
// 		return nil, err
// 	}
// 	allglenBytes, err := p.CurrentThread.readMemory(uintptr(addr), 8)
// 	if err != nil {
// 		return nil, err
// 	}
// 	allglen := binary.LittleEndian.Uint64(allglenBytes)

// 	rdr.Seek(0)
// 	allgentryaddr, err := rdr.AddrFor("runtime.allgs")
// 	if err != nil {
// 		// try old name (pre Go 1.6)
// 		allgentryaddr, err = rdr.AddrFor("runtime.allg")
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	faddr, err := p.CurrentThread.readMemory(uintptr(allgentryaddr), p.arch.PtrSize())
// 	allgptr := binary.LittleEndian.Uint64(faddr)

// 	for i := uint64(0); i < allglen; i++ {
// 		gvar, err := p.CurrentThread.newGVariable(uintptr(allgptr+(i*uint64(p.arch.PtrSize()))), true)
// 		if err != nil {
// 			return nil, err
// 		}
// 		g, err := gvar.parseG()
// 		if err != nil {
// 			return nil, err
// 		}
// 		if thread, allocated := threadg[g.ID]; allocated {
// 			loc, err := thread.Location()
// 			if err != nil {
// 				return nil, err
// 			}
// 			g.thread = thread
// 			// Prefer actual thread location information.
// 			g.CurrentLoc = *loc
// 		}
// 		if g.Status != Gdead {
// 			allg = append(allg, g)
// 		}
// 	}
// 	p.allGCache = allg
// 	return allg, nil
// }

// // Halt stops all threads.
// func (p *Process) Halt() (err error) {
// 	if p.exited {
// 		return &ProcessExitedError{}
// 	}
// 	for _, th := range p.Threads {
// 		if err := th.Halt(); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// // Registers obtains register values from the
// // "current" thread of the traced process.
// func (p *Process) Registers() (Registers, error) {
// 	return p.CurrentThread.Registers()
// }

// // PC returns the PC of the current thread.
// func (p *Process) PC() (uint64, error) {
// 	return p.CurrentThread.PC()
// }

// // CurrentBreakpoint returns the breakpoint the current thread
// // is stopped at.
// func (p *Process) CurrentBreakpoint() *Breakpoint {
// 	return p.CurrentThread.CurrentBreakpoint
// }

// // DwarfReader returns a reader for the dwarf data
// func (p *Process) DwarfReader() *reader.Reader {
// 	return reader.New(p.dwarf)
// }

// // Sources returns list of source files that comprise the debugged binary.
// func (p *Process) Sources() map[string]*gosym.Obj {
// 	return p.goSymTable.Files
// }

// // Funcs returns list of functions present in the debugged program.
// func (p *Process) Funcs() []gosym.Func {
// 	return p.goSymTable.Funcs
// }

// // Types returns list of types present in the debugged program.
// func (p *Process) Types() ([]string, error) {
// 	types := make([]string, 0, len(p.types))
// 	for k := range p.types {
// 		types = append(types, k)
// 	}
// 	return types, nil
// }

// // PCToLine converts an instruction address to a file/line/function.
// func (p *Process) PCToLine(pc uint64) (string, int, *gosym.Func) {
// 	return p.goSymTable.PCToLine(pc)
// }

// // FindBreakpointByID finds the breakpoint for the given ID.
// func (p *Process) FindBreakpointByID(id int) (*Breakpoint, bool) {
// 	for _, bp := range p.Breakpoints {
// 		if bp.ID == id {
// 			return bp, true
// 		}
// 	}
// 	return nil, false
// }

// // FindBreakpoint finds the breakpoint for the given pc.
// func (p *Process) FindBreakpoint(pc uint64) (*Breakpoint, bool) {
// 	// Check to see if address is past the breakpoint, (i.e. breakpoint was hit).
// 	if bp, ok := p.Breakpoints[pc-uint64(p.arch.BreakpointSize())]; ok {
// 		return bp, true
// 	}
// 	// Directly use addr to lookup breakpoint.
// 	if bp, ok := p.Breakpoints[pc]; ok {
// 		return bp, true
// 	}
// 	return nil, false
// }

// // Returns a new Process struct.
// func initializeDebugProcess(p *Process, path string, attach bool) (*Process, error) {
// 	if attach {
// 		var err error
// 		p.execPtraceFunc(func() { err = PtraceAttach(p.Pid) })
// 		if err != nil {
// 			return nil, err
// 		}
// 		_, _, err = p.wait(0)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	proc, err := os.FindProcess(p.Pid)
// 	if err != nil {
// 		return nil, err
// 	}

// 	p.Process = proc
// 	err = p.LoadInformation(path)
// 	if err != nil {
// 		return nil, err
// 	}

// 	switch runtime.GOARCH {
// 	case "amd64":
// 		p.arch = AMD64Arch()
// 	}

// 	if err := p.updateThreadList(); err != nil {
// 		return nil, err
// 	}

// 	ver, isextld, err := p.getGoInformation()
// 	if err != nil {
// 		return nil, err
// 	}

// 	p.arch.SetGStructOffset(ver, isextld)
// 	// SelectedGoroutine can not be set correctly by the call to updateThreadList
// 	// because without calling SetGStructOffset we can not read the G struct of CurrentThread
// 	// but without calling updateThreadList we can not examine memory to determine
// 	// the offset of g struct inside TLS
// 	p.SelectedGoroutine, _ = p.CurrentThread.GetG()

// 	panicpc, err := p.FindFunctionLocation("runtime.startpanic", true, 0)
// 	if err == nil {
// 		bp, err := p.SetBreakpoint(panicpc)
// 		if err == nil {
// 			bp.Name = "unrecovered-panic"
// 			bp.ID = -1
// 			p.breakpointIDCounter--
// 		}
// 	}

// 	return p, nil
// }

// func (p *Process) clearTempBreakpoints() error {
// 	for _, bp := range p.Breakpoints {
// 		if !bp.Temp {
// 			continue
// 		}
// 		if _, err := p.ClearBreakpoint(bp.Addr); err != nil {
// 			return err
// 		}
// 	}
// 	for i := range p.Threads {
// 		if p.Threads[i].CurrentBreakpoint != nil && p.Threads[i].CurrentBreakpoint.Temp {
// 			p.Threads[i].CurrentBreakpoint = nil
// 		}
// 	}
// 	return nil
// }

// func (p *Process) execPtraceFunc(fn func()) {
// 	p.ptraceChan <- fn
// 	<-p.ptraceDoneChan
// }

// func (p *Process) getGoInformation() (ver GoVersion, isextld bool, err error) {
// 	vv, err := p.EvalPackageVariable("runtime.buildVersion")
// 	if err != nil {
// 		err = fmt.Errorf("Could not determine version number: %v\n", err)
// 		return
// 	}
// 	if vv.Unreadable != nil {
// 		err = fmt.Errorf("Unreadable version number: %v\n", vv.Unreadable)
// 		return
// 	}

// 	ver, ok := ParseVersionString(constant.StringVal(vv.Value))
// 	if !ok {
// 		err = fmt.Errorf("Could not parse version number: %v\n", vv.Value)
// 		return
// 	}

// 	rdr := p.DwarfReader()
// 	rdr.Seek(0)
// 	for entry, err := rdr.NextCompileUnit(); entry != nil; entry, err = rdr.NextCompileUnit() {
// 		if err != nil {
// 			return ver, isextld, err
// 		}
// 		if prod, ok := entry.Val(dwarf.AttrProducer).(string); ok && (strings.HasPrefix(prod, "GNU AS")) {
// 			isextld = true
// 			break
// 		}
// 	}
// 	return
// }

// // FindGoroutine returns a G struct representing the goroutine
// // specified by `gid`.
// func (p *Process) FindGoroutine(gid int) (*G, error) {
// 	if gid == -1 {
// 		return p.SelectedGoroutine, nil
// 	}

// 	gs, err := p.GoroutinesInfo()
// 	if err != nil {
// 		return nil, err
// 	}
// 	for i := range gs {
// 		if gs[i].ID == gid {
// 			return gs[i], nil
// 		}
// 	}
// 	return nil, fmt.Errorf("Unknown goroutine %d", gid)
// }

// // ConvertEvalScope returns a new EvalScope in the context of the
// // specified goroutine ID and stack frame.
// func (p *Process) ConvertEvalScope(gid, frame int) (*EvalScope, error) {
// 	if p.exited {
// 		return nil, &ProcessExitedError{}
// 	}
// 	g, err := p.FindGoroutine(gid)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if g == nil {
// 		return p.CurrentThread.Scope()
// 	}

// 	var out EvalScope

// 	if g.thread == nil {
// 		out.Thread = p.CurrentThread
// 	} else {
// 		out.Thread = g.thread
// 	}

// 	locs, err := g.Stacktrace(frame)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if frame >= len(locs) {
// 		return nil, fmt.Errorf("Frame %d does not exist in goroutine %d", frame, gid)
// 	}

// 	out.PC, out.CFA = locs[frame].Current.PC, locs[frame].CFA

// 	return &out, nil
// }

// func (p *Process) postExit() {
// 	defer func() {
// 		if err := recover(); err != nil {
// 			log.Printf("error during postExit: %v", err)
// 		}
// 	}()
// 	p.exited = true
// 	close(p.ptraceChan)
// 	close(p.ptraceDoneChan)
// }
