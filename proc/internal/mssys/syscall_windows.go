//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go syscall_windows.go

package mssys

import (
	"syscall"
)

type NTSTATUS int32

type CLIENT_ID struct {
	UniqueProcess syscall.Handle
	UniqueThread  syscall.Handle
}

type THREAD_BASIC_INFORMATION struct {
	ExitStatus     NTSTATUS
	TebBaseAddress uintptr
	ClientId       CLIENT_ID
	AffinityMask   uintptr
	Priority       int32
	BasePriority   int32
}

type CREATE_PROCESS_DEBUG_INFO struct {
	File                syscall.Handle
	Process             syscall.Handle
	Thread              syscall.Handle
	BaseOfImage         uintptr
	DebugInfoFileOffset uint32
	DebugInfoSize       uint32
	ThreadLocalBase     uintptr
	StartAddress        uintptr
	ImageName           uintptr
	Unicode             uint16
}

type CREATE_THREAD_DEBUG_INFO struct {
	Thread          syscall.Handle
	ThreadLocalBase uintptr
	StartAddress    uintptr
}

type EXIT_PROCESS_DEBUG_INFO struct {
	ExitCode uint32
}

type LOAD_DLL_DEBUG_INFO struct {
	File                syscall.Handle
	BaseOfDll           uintptr
	DebugInfoFileOffset uint32
	DebugInfoSize       uint32
	ImageName           uintptr
	Unicode             uint16
}

const (
	ThreadBasicInformation = 0

	DBG_CONTINUE              = 0x00010002
	DBG_EXCEPTION_NOT_HANDLED = 0x80010001

	EXCEPTION_DEBUG_EVENT      = 1
	CREATE_THREAD_DEBUG_EVENT  = 2
	CREATE_PROCESS_DEBUG_EVENT = 3
	EXIT_THREAD_DEBUG_EVENT    = 4
	EXIT_PROCESS_DEBUG_EVENT   = 5
	LOAD_DLL_DEBUG_EVENT       = 6
	UNLOAD_DLL_DEBUG_EVENT     = 7
	OUTPUT_DEBUG_STRING_EVENT  = 8
	RIP_EVENT                  = 9
)

func NT_SUCCESS(x NTSTATUS) bool {
	return x >= 0
}

//sys	NtQueryInformationThread(threadHandle syscall.Handle, infoclass int32, info uintptr, infolen uint32, retlen *uint32) (status NTSTATUS) = ntdll.NtQueryInformationThread
//sys	GetThreadContext(thread syscall.Handle, context *_CONTEXT) (err error) = kernel32.GetThreadContext
//sys	SetThreadContext(thread syscall.Handle, context *_CONTEXT) (err error) = kernel32.SetThreadContext
//sys	SuspendThread(threadid syscall.Handle) (prevsuspcount uint32, err error) [failretval==0xffffffff] = kernel32.SuspendThread
//sys	ResumeThread(threadid syscall.Handle) (prevsuspcount uint32, err error) [failretval==0xffffffff] = kernel32.ResumeThread
//sys	ContinueDebugEvent(processid uint32, threadid uint32, continuestatus uint32) (err error) = kernel32.ContinueDebugEvent
//sys	WriteProcessMemory(process syscall.Handle, baseaddr uintptr, buffer *byte, size uintptr, byteswritten *uintptr) (err error) = kernel32.WriteProcessMemory
//sys	ReadProcessMemory(process syscall.Handle, baseaddr uintptr, buffer *byte, size uintptr, bytesread *uintptr) (err error) = kernel32.ReadProcessMemory
//sys	DebugBreakProcess(process syscall.Handle) (err error) = kernel32.DebugBreakProcess
//sys	WaitForDebugEvent(debugevent *_DEBUG_EVENT, milliseconds uint32) (err error) = kernel32.WaitForDebugEvent
