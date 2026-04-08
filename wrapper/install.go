package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	ifeoPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
)

var targetExes = []string{"stalcraft.exe", "stalcraftw.exe"}

func install() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[install] Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	for _, target := range targetExes {
		key, _, err := registry.CreateKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.ALL_ACCESS,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[install] Failed to create IFEO key for %s (run as admin): %v\n", target, err)
			os.Exit(1)
		}

		if err := key.SetStringValue("Debugger", `"`+self+`"`); err != nil {
			key.Close()
			fmt.Fprintf(os.Stderr, "[install] Failed to set Debugger value for %s: %v\n", target, err)
			os.Exit(1)
		}
		key.Close()

		fmt.Printf("[install] IFEO registered for %s\n", target)
	}
	fmt.Printf("[install] Debugger = %s\n", self)
}

func uninstall() {
	for _, target := range targetExes {
		key, err := registry.OpenKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.SET_VALUE,
		)
		if err != nil {
			continue
		}

		if err := key.DeleteValue("Debugger"); err != nil {
			key.Close()
			continue
		}
		key.Close()

		fmt.Printf("[uninstall] IFEO removed for %s\n", target)
	}
}

func status() {
	found := false
	for _, target := range targetExes {
		key, err := registry.OpenKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.QUERY_VALUE,
		)
		if err != nil {
			fmt.Printf("[status] %s: not installed\n", target)
			continue
		}

		val, _, err := key.GetStringValue("Debugger")
		key.Close()
		if err != nil {
			fmt.Printf("[status] %s: not installed\n", target)
			continue
		}

		fmt.Printf("[status] %s -> %s\n", target, val)
		found = true
	}
	if !found {
		fmt.Println("[status] Not installed")
	}
}
