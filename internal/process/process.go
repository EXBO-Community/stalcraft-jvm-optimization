// Package process launches the game executable via NtCreateUserProcess so we
// can set PS_ATTRIBUTE_IFEO_SKIP_DEBUGGER and escape our own IFEO hook, then
// boosts the child's priorities and waits until the first visible window
// appears (or until it exits).
package process

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

var (
	procRtlCreateProcessParametersEx = winapi.Ntdll.NewProc("RtlCreateProcessParametersEx")
	procNtCreateUserProcess          = winapi.Ntdll.NewProc("NtCreateUserProcess")
	procRtlDestroyProcessParameters  = winapi.Ntdll.NewProc("RtlDestroyProcessParameters")
	procNtSetInformationProcess      = winapi.Ntdll.NewProc("NtSetInformationProcess")

	procSetProcessPriorityBoost = winapi.Kernel32.NewProc("SetProcessPriorityBoost")
	procGetExitCodeProcess      = winapi.Kernel32.NewProc("GetExitCodeProcess")

	procEnumWindows              = winapi.User32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = winapi.User32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = winapi.User32.NewProc("IsWindowVisible")
)

const (
	psAttrImageName                  = 0x00020005
	psAttrClientID                   = 0x00010003
	rtlUserProcParamsNormalized      = 0x01
	ifeoSkipDebugger                 = 0x04
	processCreateFlagsInheritHandles = 0x04
	processAllAccess                 = 0x001FFFFF
	threadAllAccess                  = 0x001FFFFF

	processMemoryPriority = 0x27
	processIoPriority     = 0x21
	memoryPriorityHigh    = 5
	ioPriorityHigh        = 3
)

// Process is a handle pair produced by Start. Always Close it.
type Process struct {
	Handle syscall.Handle
	Thread syscall.Handle
	PID    uint32
}

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	_             [4]byte
	Buffer        *uint16
}

type clientID struct {
	UniqueProcess uintptr
	UniqueThread  uintptr
}

type psAttribute struct {
	Attribute    uintptr
	Size         uintptr
	Value        uintptr
	ReturnLength uintptr
}

type psAttributeList2 struct {
	TotalLength uintptr
	Attributes  [2]psAttribute
}

type psCreateInfo [0x58]byte

func newUnicodeString(s string) (unicodeString, []uint16, error) {
	buf, err := syscall.UTF16FromString(s)
	if err != nil {
		return unicodeString{}, nil, fmt.Errorf("encode %q: %w", s, err)
	}
	return unicodeString{
		Length:        uint16((len(buf) - 1) * 2),
		MaximumLength: uint16(len(buf) * 2),
		Buffer:        &buf[0],
	}, buf, nil
}

func envBlock() []uint16 {
	var block []uint16
	for _, e := range os.Environ() {
		s, err := syscall.UTF16FromString(e)
		if err != nil {
			continue
		}
		block = append(block, s...)
	}
	return append(block, 0)
}

func buildCmdLine(exe string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, `"`+exe+`"`)
	for _, a := range args {
		if strings.ContainsAny(a, ` "`) {
			parts = append(parts, `"`+a+`"`)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// extractGameDir derives the working directory from --gameDir or
// -Djava.library.path so the child process sees its resources.
func extractGameDir(exePath string, args []string) string {
	for i, a := range args {
		if a == "--gameDir" && i+1 < len(args) {
			return args[i+1]
		}
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-Djava.library.path=") {
			libPath := filepath.ToSlash(strings.TrimPrefix(a, "-Djava.library.path="))
			exeDir := filepath.ToSlash(filepath.Dir(exePath))
			if strings.HasSuffix(exeDir, libPath) {
				return filepath.FromSlash(exeDir[:len(exeDir)-len(libPath)])
			}
		}
	}
	return ""
}

// resolveWorkDir returns the absolute working directory for the spawned
// game process: an explicit --gameDir / -Djava.library.path value when
// present, or the directory containing the target executable as a
// fallback.
func resolveWorkDir(exePath string, args []string) string {
	abs, err := filepath.Abs(exePath)
	if err != nil {
		abs = exePath
	}
	dir := extractGameDir(abs, args)
	if dir == "" {
		dir = filepath.Dir(abs)
	}
	if absDir, err := filepath.Abs(dir); err == nil {
		return absDir
	}
	return dir
}

// Start spawns exePath with args via NtCreateUserProcess.
// The resulting Process must be closed by the caller.
func Start(exePath string, args []string) (*Process, error) {
	absPath, err := filepath.Abs(exePath)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", exePath, err)
	}
	ntPath := `\??\` + absPath
	cmdLine := buildCmdLine(absPath, args)

	workDir := resolveWorkDir(exePath, args)

	imgUS, imgBuf, err := newUnicodeString(absPath)
	if err != nil {
		return nil, err
	}
	cmdUS, cmdBuf, err := newUnicodeString(cmdLine)
	if err != nil {
		return nil, err
	}
	wdUS, wdBuf, err := newUnicodeString(workDir)
	if err != nil {
		return nil, err
	}
	ntUS, ntBuf, err := newUnicodeString(ntPath)
	if err != nil {
		return nil, err
	}
	desktopUS, desktopBuf, err := newUnicodeString(`WinSta0\Default`)
	if err != nil {
		return nil, err
	}
	env := envBlock()

	var params uintptr
	status, _, _ := procRtlCreateProcessParametersEx.Call(
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&imgUS)),
		0,
		uintptr(unsafe.Pointer(&wdUS)),
		uintptr(unsafe.Pointer(&cmdUS)),
		uintptr(unsafe.Pointer(&env[0])),
		0,
		uintptr(unsafe.Pointer(&desktopUS)),
		0, 0,
		rtlUserProcParamsNormalized,
	)
	if status != 0 {
		return nil, fmt.Errorf("RtlCreateProcessParametersEx: NTSTATUS 0x%08x", status)
	}
	defer procRtlDestroyProcessParameters.Call(params)

	var ci psCreateInfo
	*(*uintptr)(unsafe.Pointer(&ci[0])) = 0x58
	*(*uint32)(unsafe.Pointer(&ci[0x10])) = ifeoSkipDebugger

	var cid clientID
	al := psAttributeList2{
		TotalLength: unsafe.Sizeof(psAttributeList2{}),
		Attributes: [2]psAttribute{
			{
				Attribute: psAttrImageName,
				Size:      uintptr(ntUS.Length),
				Value:     uintptr(unsafe.Pointer(ntUS.Buffer)),
			},
			{
				Attribute: psAttrClientID,
				Size:      unsafe.Sizeof(cid),
				Value:     uintptr(unsafe.Pointer(&cid)),
			},
		},
	}

	var hProcess, hThread syscall.Handle
	status, _, _ = procNtCreateUserProcess.Call(
		uintptr(unsafe.Pointer(&hProcess)),
		uintptr(unsafe.Pointer(&hThread)),
		processAllAccess, threadAllAccess,
		0, 0,
		processCreateFlagsInheritHandles,
		0,
		params,
		uintptr(unsafe.Pointer(&ci)),
		uintptr(unsafe.Pointer(&al)),
	)

	// Keep the UTF-16 buffers alive across the syscall — NtCreateUserProcess
	// reads the fields referenced by params/attribute list.
	runtime.KeepAlive(imgBuf)
	runtime.KeepAlive(cmdBuf)
	runtime.KeepAlive(wdBuf)
	runtime.KeepAlive(ntBuf)
	runtime.KeepAlive(env)
	runtime.KeepAlive(desktopBuf)
	runtime.KeepAlive(&cid)

	if status != 0 {
		return nil, fmt.Errorf("NtCreateUserProcess: NTSTATUS 0x%08x", status)
	}

	return &Process{
		Handle: hProcess,
		Thread: hThread,
		PID:    uint32(cid.UniqueProcess),
	}, nil
}

// Close releases the process and thread handles.
func (p *Process) Close() {
	if p == nil {
		return
	}
	if p.Handle != 0 {
		syscall.CloseHandle(p.Handle)
		p.Handle = 0
	}
	if p.Thread != 0 {
		syscall.CloseHandle(p.Thread)
		p.Thread = 0
	}
}

// Boost raises memory/IO priority and disables dynamic priority decay.
// Returns the first failure it encounters; callers can ignore errors
// since this is a best-effort tuning step.
func (p *Process) Boost() error {
	var errs []error
	if ret, _, err := procSetProcessPriorityBoost.Call(uintptr(p.Handle), 1); ret == 0 {
		errs = append(errs, fmt.Errorf("SetProcessPriorityBoost: %w", err))
	}

	mem := uint32(memoryPriorityHigh)
	if status, _, _ := procNtSetInformationProcess.Call(
		uintptr(p.Handle), processMemoryPriority,
		uintptr(unsafe.Pointer(&mem)), unsafe.Sizeof(mem),
	); status != 0 {
		errs = append(errs, fmt.Errorf("NtSetInformationProcess(memory): NTSTATUS 0x%08x", status))
	}

	iop := uint32(ioPriorityHigh)
	if status, _, _ := procNtSetInformationProcess.Call(
		uintptr(p.Handle), processIoPriority,
		uintptr(unsafe.Pointer(&iop)), unsafe.Sizeof(iop),
	); status != 0 {
		errs = append(errs, fmt.Errorf("NtSetInformationProcess(io): NTSTATUS 0x%08x", status))
	}
	return errors.Join(errs...)
}

// Wait polls the process until it exits or until it shows a visible window.
// Returning on window visibility lets the wrapper quit early so the
// console window detaches from the game.
func (p *Process) Wait() (exitCode int, err error) {
	for {
		ret, waitErr := syscall.WaitForSingleObject(p.Handle, 200)
		if waitErr != nil {
			return 1, fmt.Errorf("WaitForSingleObject: %w", waitErr)
		}
		if ret == 0 {
			var code uint32
			if r, _, callErr := procGetExitCodeProcess.Call(uintptr(p.Handle), uintptr(unsafe.Pointer(&code))); r == 0 {
				return 1, fmt.Errorf("GetExitCodeProcess: %w", callErr)
			}
			return int(code), nil
		}
		if hasVisibleWindow(p.PID) {
			slog.Info("process visible window, detaching", "pid", p.PID)
			return 0, nil
		}
	}
}

// windowSearch carries per-call state for the EnumWindows callback.
// It lives in a small sync.Map registry keyed by a monotonic uintptr
// handle, which is what we feed EnumWindows as LPARAM. This is a pure
// Go equivalent of runtime/cgo.Handle — we can't import that one
// because it forces CGO_ENABLED=1 and we cross-compile from macOS.
// The approach is vet-clean (no unsafe.Pointer smuggling a Go value
// through uintptr) and supports concurrent hasVisibleWindow calls
// without any shared mutable state.
type windowSearch struct {
	targetPID uint32
	found     bool
}

var (
	searchHandleSeq atomic.Uintptr
	searchHandles   sync.Map // uintptr → *windowSearch
)

var enumCallback = syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
	v, ok := searchHandles.Load(lparam)
	if !ok {
		return 1
	}
	s := v.(*windowSearch)

	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == s.targetPID {
		if vis, _, _ := procIsWindowVisible.Call(hwnd); vis != 0 {
			s.found = true
			return 0
		}
	}
	return 1
})

func hasVisibleWindow(pid uint32) bool {
	s := &windowSearch{targetPID: pid}
	key := searchHandleSeq.Add(1)
	searchHandles.Store(key, s)
	defer searchHandles.Delete(key)

	procEnumWindows.Call(enumCallback, key)
	return s.found
}
