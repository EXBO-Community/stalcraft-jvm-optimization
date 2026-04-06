package main

import (
	"os"
	"strings"
	"syscall"
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

func hideConsole() {
	hwnd, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
	if hwnd != 0 {
		user32.NewProc("ShowWindow").Call(hwnd, 0)
	}
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
	return waitProcess(hProcess)
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

	hideConsole()
	os.Exit(run())
}
