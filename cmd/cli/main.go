// Command cli is the user-facing entry point of the STALZONE JVM
// optimization wrapper. It renders the interactive menu and handles
// the install/uninstall/status CLI flags. The actual IFEO Debugger
// that hooks the game launch lives in the sibling binary service.exe.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/installer"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/logging"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/ui"
)

func main() {
	closeLog, err := logging.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[log] %v\n", err)
	}
	defer closeLog()

	slog.Info("cli startup", "args_count", len(os.Args)-1)

	if handled, code := handleCLI(); handled {
		os.Exit(code)
	}

	if err := ui.Run(); err != nil {
		slog.Error("ui failed", "err", err)
		fmt.Fprintf(os.Stderr, "[ui] %v\n", err)
		os.Exit(1)
	}
}

func handleCLI() (handled bool, code int) {
	if len(os.Args) < 2 {
		return false, 0
	}
	switch os.Args[1] {
	case "--install":
		if err := installer.Install(); err != nil {
			slog.Error("install failed", "err", err)
			fmt.Fprintf(os.Stderr, "[install] %v\n", err)
			return true, 1
		}
		return true, 0
	case "--uninstall":
		if err := installer.Uninstall(); err != nil {
			slog.Error("uninstall failed", "err", err)
			fmt.Fprintf(os.Stderr, "[uninstall] %v\n", err)
			return true, 1
		}
		return true, 0
	case "--status":
		ui.PrintStatus()
		return true, 0
	}
	return false, 0
}
