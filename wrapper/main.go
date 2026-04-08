package main

import (
	"os"
	"syscall"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
)

func run() int {
	ensureConfigExists()

	cfg, ok := loadActiveConfig()

	var args []string
	if !ok || cfg.HeapSizeGB == 0 {
		args = os.Args[2:]
	} else {
		args = filterArgs(os.Args[2:], generateFlagsFromConfig(cfg))
	}

	hProcess, hThread, pid, err := ntCreateProcess(os.Args[1], args)
	if err != nil {
		return 1
	}
	defer syscall.CloseHandle(hProcess)
	defer syscall.CloseHandle(hThread)

	boostProcess(hProcess)
	return waitProcess(hProcess, pid)
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--install":
			install()
			return
		case "--uninstall":
			uninstall()
			return
		case "--status":
			status()
			return
		}
	}

	if len(os.Args) < 2 {
		interactiveMenu()
		return
	}

	createPhantomWindow()
	os.Exit(run())
}
