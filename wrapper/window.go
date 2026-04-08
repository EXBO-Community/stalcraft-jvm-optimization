package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

const (
	wsVisible      = 0x10000000
	wsPopup        = 0x80000000
	wsExToolWindow = 0x00000080
	wsExLayered    = 0x00080000
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

func createPhantomWindow() {
	go func() {
		runtime.LockOSThread()

		className, _ := syscall.UTF16PtrFromString("StalcraftWrapper")
		defWindowProc := user32.NewProc("DefWindowProcW")

		wc := wndClassExW{
			Size:      uint32(unsafe.Sizeof(wndClassExW{})),
			WndProc:   defWindowProc.Addr(),
			ClassName: className,
		}
		user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
			wsExToolWindow|wsExLayered,
			uintptr(unsafe.Pointer(className)), 0,
			wsVisible|wsPopup,
			0, 0, 0, 0, 0, 0, 0, 0,
		)
		user32.NewProc("SetLayeredWindowAttributes").Call(hwnd, 0, 0, 0x02)

		var m msg
		getMessage := user32.NewProc("GetMessageW")
		translateMessage := user32.NewProc("TranslateMessage")
		dispatchMessage := user32.NewProc("DispatchMessageW")
		for {
			ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if ret == 0 || ret == ^uintptr(0) {
				break
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&m)))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
}
