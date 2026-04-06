package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// NT API procedures
var (
	procRtlCreateProcessParametersEx = ntdll.NewProc("RtlCreateProcessParametersEx")
	procNtCreateUserProcess          = ntdll.NewProc("NtCreateUserProcess")
	procRtlDestroyProcessParameters  = ntdll.NewProc("RtlDestroyProcessParameters")
	procSetProcessPriorityBoost      = kernel32.NewProc("SetProcessPriorityBoost")
	procNtSetInformationProcess      = ntdll.NewProc("NtSetInformationProcess")
	procGetExitCodeProcess           = kernel32.NewProc("GetExitCodeProcess")
)

// NT constants
const (
	psAttrImageName                  = 0x00020005
	psAttrClientID                   = 0x00010003
	rtlUserProcParamsNormalized      = 0x01
	ifeoSkipDebugger                 = 0x04
	processCreateFlagsInheritHandles = 0x04
	processAllAccess                 = 0x001FFFFF
	threadAllAccess                  = 0x001FFFFF
	processSetInfo                   = 0x0200
	processQueryInfo                 = 0x0400
	processMemoryPriority            = 0x27
	processIoPriority                = 0x21
	memoryPriorityHigh               = 5
	ioPriorityHigh                   = 3
	infinite                         = 0xFFFFFFFF
)

// UNICODE_STRING (x64: 16 bytes)
type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	_             [4]byte
	Buffer        *uint16
}

// CLIENT_ID (x64: 16 bytes)
type clientID struct {
	UniqueProcess uintptr
	UniqueThread  uintptr
}

// PS_ATTRIBUTE (x64: 32 bytes)
type psAttribute struct {
	Attribute    uintptr
	Size         uintptr
	Value        uintptr
	ReturnLength uintptr
}

// PS_ATTRIBUTE_LIST for 2 attributes (ImageName + ClientId)
type psAttributeList2 struct {
	TotalLength uintptr
	Attributes  [2]psAttribute
}

// PS_CREATE_INFO (x64: 0x58 bytes, stable across Win10/Win11)
type psCreateInfo [0x58]byte

func newUnicodeString(s string) (unicodeString, []uint16) {
	buf, _ := syscall.UTF16FromString(s)
	return unicodeString{
		Length:        uint16((len(buf) - 1) * 2),
		MaximumLength: uint16(len(buf) * 2),
		Buffer:        &buf[0],
	}, buf
}

func createEnvBlock() []uint16 {
	var block []uint16
	for _, e := range os.Environ() {
		s, _ := syscall.UTF16FromString(e)
		block = append(block, s...)
	}
	return append(block, 0)
}

func buildCmdLine(exe string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, `"`+exe+`"`)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func extractGameDir(args []string) string {
	for i, a := range args {
		if a == "--gameDir" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// ntCreateProcess creates a process via NtCreateUserProcess, bypassing kernel32 IFEO check.
func ntCreateProcess(exePath string, args []string) (hProcess, hThread syscall.Handle, pid uint32, err error) {
	absPath, _ := filepath.Abs(exePath)
	ntPath := `\??\` + absPath
	cmdLine := buildCmdLine(absPath, args)

	workDir := extractGameDir(args)
	if workDir == "" {
		workDir = filepath.Dir(absPath)
	}

	imgUS, imgBuf := newUnicodeString(absPath)
	cmdUS, cmdBuf := newUnicodeString(cmdLine)
	wdUS, wdBuf := newUnicodeString(workDir)
	ntUS, ntBuf := newUnicodeString(ntPath)
	envBlock := createEnvBlock()
	desktopUS, desktopBuf := newUnicodeString(`WinSta0\Default`)

	var params uintptr
	r, _, _ := procRtlCreateProcessParametersEx.Call(
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&imgUS)),
		0, // DllPath
		uintptr(unsafe.Pointer(&wdUS)),
		uintptr(unsafe.Pointer(&cmdUS)),
		uintptr(unsafe.Pointer(&envBlock[0])),
		0, // WindowTitle
		uintptr(unsafe.Pointer(&desktopUS)),
		0, 0, // ShellInfo, RuntimeData
		rtlUserProcParamsNormalized,
	)
	if r != 0 {
		err = fmt.Errorf("RtlCreateProcessParametersEx: 0x%08x", r)
		return
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

	r, _, _ = procNtCreateUserProcess.Call(
		uintptr(unsafe.Pointer(&hProcess)),
		uintptr(unsafe.Pointer(&hThread)),
		processAllAccess, threadAllAccess,
		0, 0, // ObjectAttributes
		processCreateFlagsInheritHandles,
		0, // ThreadFlags
		params,
		uintptr(unsafe.Pointer(&ci)),
		uintptr(unsafe.Pointer(&al)),
	)

	runtime.KeepAlive(imgBuf)
	runtime.KeepAlive(cmdBuf)
	runtime.KeepAlive(wdBuf)
	runtime.KeepAlive(ntBuf)
	runtime.KeepAlive(envBlock)
	runtime.KeepAlive(desktopBuf)
	runtime.KeepAlive(&cid)

	if r != 0 {
		err = fmt.Errorf("NtCreateUserProcess: 0x%08x", r)
		return
	}

	pid = uint32(cid.UniqueProcess)
	return
}

// boostProcess sets high memory/IO priority and disables priority decay.
func boostProcess(pid uint32) {
	handle, err := syscall.OpenProcess(processSetInfo|processQueryInfo, false, pid)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)

	procSetProcessPriorityBoost.Call(uintptr(handle), 1)

	mem := uint32(memoryPriorityHigh)
	procNtSetInformationProcess.Call(
		uintptr(handle), processMemoryPriority,
		uintptr(unsafe.Pointer(&mem)), unsafe.Sizeof(mem),
	)

	iop := uint32(ioPriorityHigh)
	procNtSetInformationProcess.Call(
		uintptr(handle), processIoPriority,
		uintptr(unsafe.Pointer(&iop)), unsafe.Sizeof(iop),
	)
}

func waitProcess(hProcess syscall.Handle) int {
	syscall.WaitForSingleObject(hProcess, infinite)
	var exitCode uint32
	procGetExitCodeProcess.Call(uintptr(hProcess), uintptr(unsafe.Pointer(&exitCode)))
	return int(exitCode)
}
