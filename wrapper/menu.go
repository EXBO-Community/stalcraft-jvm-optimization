package main

import (
	"fmt"
	"os"
	"unsafe"
)

var (
	procReadConsoleInput = kernel32.NewProc("ReadConsoleInputW")
	procGetStdHandle     = kernel32.NewProc("GetStdHandle")
	procSetConsoleMode   = kernel32.NewProc("SetConsoleMode")
	procGetConsoleMode   = kernel32.NewProc("GetConsoleMode")
)

const (
	stdInputHandle  = ^uintptr(10 - 1) // -10
	stdOutputHandle = ^uintptr(11 - 1) // -11

	enableEchoInput               = 0x0004
	enableLineInput               = 0x0002
	enableVirtualTerminalProcessing = 0x0004

	keyEvent = 0x0001
)

type inputRecord struct {
	EventType uint16
	_         uint16
	KeyDown   int32
	RepCount  uint16
	VKeyCode  uint16
	VScanCode uint16
	Char      uint16
	CtrlState uint32
}

type menuItem struct {
	label  string
	action func()
}

func enableVT() func() {
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var mode uint32
	procGetConsoleMode.Call(hOut, uintptr(unsafe.Pointer(&mode)))
	procSetConsoleMode.Call(hOut, uintptr(mode|enableVirtualTerminalProcessing))
	return func() { procSetConsoleMode.Call(hOut, uintptr(mode)) }
}

func interactiveMenu() {
	restoreVT := enableVT()
	defer restoreVT()

	fmt.Println("STALCRAFT JVM Optimization Wrapper")
	fmt.Println("-----------------------------------")
	fmt.Println("RU: Используйте стрелки для выбора, Enter для подтверждения.")
	fmt.Println("    Install  \u2014 установить оптимизацию (требуются права админа)")
	fmt.Println("    Uninstall \u2014 удалить оптимизацию")
	fmt.Println("    Status   \u2014 проверить статус установки")
	fmt.Println()
	fmt.Println("EN: Use arrow keys to select, Enter to confirm.")
	fmt.Println("    Install   \u2014 enable JVM optimization (requires admin)")
	fmt.Println("    Uninstall \u2014 remove JVM optimization")
	fmt.Println("    Status    \u2014 check installation status")
	fmt.Println()

	items := []menuItem{
		{"Install", install},
		{"Uninstall", uninstall},
		{"Status", status},
		{"Exit", func() { os.Exit(0) }},
	}

	hIn, _, _ := procGetStdHandle.Call(stdInputHandle)
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)

	// Hide cursor
	cursorInfo := [2]uint32{100, 0} // size=100, visible=FALSE
	kernel32.NewProc("SetConsoleCursorInfo").Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	defer func() {
		cursorInfo[1] = 1 // visible=TRUE
		kernel32.NewProc("SetConsoleCursorInfo").Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	}()

	var oldMode uint32
	procGetConsoleMode.Call(hIn, uintptr(unsafe.Pointer(&oldMode)))
	procSetConsoleMode.Call(hIn, uintptr(oldMode&^(enableLineInput|enableEchoInput)))
	defer procSetConsoleMode.Call(hIn, uintptr(oldMode))

	selected := 0
	drawMenu(items, selected)

	for {
		vk := readKey(hIn)
		switch vk {
		case 0x26: // VK_UP
			if selected > 0 {
				selected--
			}
		case 0x28: // VK_DOWN
			if selected < len(items)-1 {
				selected++
			}
		case 0x0D: // VK_RETURN
			clearMenu(len(items))
			procSetConsoleMode.Call(hIn, uintptr(oldMode))
			items[selected].action()
			fmt.Print("\nPress Enter to exit...")
			fmt.Scanln()
			return
		case 0x1B: // VK_ESCAPE
			clearMenu(len(items))
			return
		default:
			continue
		}
		drawMenu(items, selected)
	}
}

func drawMenu(items []menuItem, selected int) {
	for i := range items {
		fmt.Print("\033[2K\r") // clear line, carriage return
		if i == selected {
			fmt.Printf("  > %s", items[i].label)
		} else {
			fmt.Printf("    %s", items[i].label)
		}
		if i < len(items)-1 {
			fmt.Print("\n")
		}
	}
	// Move cursor back to first line
	fmt.Printf("\033[%dA\r", len(items)-1)
}

func clearMenu(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[2K\r")
		if i < n-1 {
			fmt.Print("\n")
		}
	}
	fmt.Printf("\033[%dA\r", n-1)
}

func readKey(hIn uintptr) uint16 {
	for {
		var rec inputRecord
		var read uint32
		procReadConsoleInput.Call(
			hIn,
			uintptr(unsafe.Pointer(&rec)),
			1,
			uintptr(unsafe.Pointer(&read)),
		)
		if read > 0 && rec.EventType == keyEvent && rec.KeyDown != 0 {
			return rec.VKeyCode
		}
	}
}
