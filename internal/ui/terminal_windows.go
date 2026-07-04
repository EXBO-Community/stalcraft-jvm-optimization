package ui

import (
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

const (
	stdOutputHandle = ^uintptr(11 - 1)
	minConsoleCols  = 96
	minConsoleRows  = 30
)

var (
	procGetStdHandle               = winapi.Kernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBufferInfo = winapi.Kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleScreenBufferSize = winapi.Kernel32.NewProc("SetConsoleScreenBufferSize")
	procSetConsoleWindowInfo       = winapi.Kernel32.NewProc("SetConsoleWindowInfo")
	procSetConsoleTitleW           = winapi.Kernel32.NewProc("SetConsoleTitleW")
)

type coord struct {
	x int16
	y int16
}

type smallRect struct {
	left   int16
	top    int16
	right  int16
	bottom int16
}

type consoleScreenBufferInfo struct {
	size              coord
	cursorPosition    coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
}

func prepareTerminal() {
	setConsoleTitle("EXBO Community")
	ensureConsoleSize(minConsoleCols, minConsoleRows)
}

func setConsoleTitle(title string) {
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(ptr)))
}

func ensureConsoleSize(minCols, minRows int16) {
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)
	if hOut == 0 || hOut == ^uintptr(0) {
		return
	}

	var info consoleScreenBufferInfo
	if ret, _, _ := procGetConsoleScreenBufferInfo.Call(
		hOut,
		uintptr(unsafe.Pointer(&info)),
	); ret == 0 {
		return
	}

	cols := maxInt16(windowWidth(info.window), minCols)
	rows := maxInt16(windowHeight(info.window), minRows)
	if info.maximumWindowSize.x > 0 && cols > info.maximumWindowSize.x {
		cols = info.maximumWindowSize.x
	}
	if info.maximumWindowSize.y > 0 && rows > info.maximumWindowSize.y {
		rows = info.maximumWindowSize.y
	}

	bufferCols := maxInt16(info.size.x, cols)
	bufferRows := maxInt16(info.size.y, rows)
	procSetConsoleScreenBufferSize.Call(hOut, coordToUintptr(coord{x: bufferCols, y: bufferRows}))

	rect := smallRect{right: cols - 1, bottom: rows - 1}
	procSetConsoleWindowInfo.Call(hOut, 1, uintptr(unsafe.Pointer(&rect)))
}

func windowWidth(r smallRect) int16 {
	return r.right - r.left + 1
}

func windowHeight(r smallRect) int16 {
	return r.bottom - r.top + 1
}

func maxInt16(a, b int16) int16 {
	if a > b {
		return a
	}
	return b
}

func coordToUintptr(c coord) uintptr {
	return uintptr(uint32(uint16(c.x)) | uint32(uint16(c.y))<<16)
}
