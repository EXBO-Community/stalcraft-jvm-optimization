// Package elevate re-launches the current executable under a UAC prompt
// so install actions can run with administrator rights.
package elevate

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

var (
	procShellExecuteExW    = winapi.Shell32.NewProc("ShellExecuteExW")
	procGetExitCodeProcess = winapi.Kernel32.NewProc("GetExitCodeProcess")
)

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

// Run restarts this executable with the given CLI arguments under
// an elevated token and waits for it to finish.
func Run(args string) (exitCode int, err error) {
	self, err := os.Executable()
	if err != nil {
		return 1, fmt.Errorf("resolve self: %w", err)
	}

	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return 1, fmt.Errorf("encode verb: %w", err)
	}
	file, err := syscall.UTF16PtrFromString(self)
	if err != nil {
		return 1, fmt.Errorf("encode self: %w", err)
	}
	params, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return 1, fmt.Errorf("encode args: %w", err)
	}

	sei := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	if ret, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei))); ret == 0 {
		return 1, fmt.Errorf("ShellExecuteEx: %w", callErr)
	}
	defer syscall.CloseHandle(sei.hProcess)

	if _, err := syscall.WaitForSingleObject(sei.hProcess, syscall.INFINITE); err != nil {
		return 1, fmt.Errorf("wait elevated process: %w", err)
	}

	var code uint32
	if r, _, callErr := procGetExitCodeProcess.Call(uintptr(sei.hProcess), uintptr(unsafe.Pointer(&code))); r == 0 {
		return 1, fmt.Errorf("GetExitCodeProcess: %w", callErr)
	}
	return int(code), nil
}
