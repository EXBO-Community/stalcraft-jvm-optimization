// Command cli is the user-facing entry point of the STALZONE JVM
// optimization wrapper. It renders the interactive menu and handles
// command-line operations. The actual IFEO Debugger
// that hooks the game launch lives in the sibling binary service.exe.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/logging"
)

func main() {
	closeLog, err := logging.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[log] %v\n", err)
	}
	defer closeLog()

	slog.Info("cli startup", "args_count", len(os.Args)-1)

	if err := newRootCommand().Execute(); err != nil {
		slog.Error("cli failed", "err", err)
		fmt.Fprintf(os.Stderr, "[cli] %v\n", err)
		os.Exit(1)
	}
}
