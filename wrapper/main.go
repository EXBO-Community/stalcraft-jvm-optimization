package main

import (
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
)

var exactRemove = map[string]bool{
	"-XX:-PrintCommandLineFlags": true,
	"-XX:+UseG1GC":               true,
}

var prefixRemove = []string{
	"-XX:MaxGCPauseMillis=",
	"-XX:MetaspaceSize=",
	"-XX:MaxMetaspaceSize=",
	"-XX:G1HeapRegionSize=",
	"-XX:G1NewSizePercent=",
	"-XX:G1MaxNewSizePercent=",
	"-XX:G1ReservePercent=",
	"-XX:G1HeapWastePercent=",
	"-XX:G1MixedGCCountTarget=",
	"-XX:InitiatingHeapOccupancyPercent=",
	"-XX:G1MixedGCLiveThresholdPercent=",
	"-XX:G1RSetUpdatingPauseTimePercent=",
	"-XX:SurvivorRatio=",
	"-XX:MaxTenuringThreshold=",
	"-XX:ParallelGCThreads=",
	"-XX:ConcGCThreads=",
	"-XX:SoftRefLRUPolicyMSPerMB=",
	"-XX:ReservedCodeCacheSize=",
	"-XX:NonNMethodCodeHeapSize=",
	"-XX:ProfiledCodeHeapSize=",
	"-XX:NonProfiledCodeHeapSize=",
	"-XX:MaxInlineLevel=",
	"-XX:FreqInlineSize=",
	"-XX:LargePageSizeInBytes=",
	"-Xms",
	"-Xmx",
}

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

func splitArgs(args []string) (jvm []string, mainClass string, app []string) {
	for i := 0; i < len(args); {
		a := args[i]
		if a == "-classpath" || a == "-cp" || a == "-jar" {
			jvm = append(jvm, a)
			i++
			if i < len(args) {
				jvm = append(jvm, args[i])
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			jvm = append(jvm, a)
			i++
			continue
		}
		mainClass = a
		app = args[i+1:]
		return
	}
	return
}

func shouldRemove(arg string) bool {
	if exactRemove[arg] {
		return true
	}
	for _, p := range prefixRemove {
		if strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

func filterArgs(orig, injected []string) []string {
	jvm, mainClass, app := splitArgs(orig)

	var filtered []string
	for _, a := range jvm {
		if !shouldRemove(a) {
			filtered = append(filtered, a)
		}
	}
	result := make([]string, 0, len(filtered)+len(injected)+1+len(app))
	result = append(result, filtered...)
	result = append(result, injected...)
	if mainClass != "" {
		result = append(result, mainClass)
	}
	return append(result, app...)
}

func run() int {
	sys := detectSystem()

	var args []string
	if calcHeap(sys) == 0 {
		args = os.Args[2:]
	} else {
		args = filterArgs(os.Args[2:], generateFlags(sys))
	}

	hProcess, hThread, pid, err := ntCreateProcess(os.Args[1], args)
	if err != nil {
		return 1
	}
	defer syscall.CloseHandle(hProcess)
	defer syscall.CloseHandle(hThread)

	boostProcess(pid)
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
