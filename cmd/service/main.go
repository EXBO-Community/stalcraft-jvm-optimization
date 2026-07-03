// Command service is the IFEO Debugger binary. Windows spawns it with
// "stalzone.exe <args>..." when the game launcher tries to start the real
// executable; service.exe then replaces the JVM flags with a tuned profile,
// starts the game via NtCreateUserProcess (bypassing
// its own IFEO hook) and waits until the game window is visible.
//
// service.exe has no UI and no installer — those live in cli.exe.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/jvm"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/logging"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/phantom"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/process"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

func main() {
	closeLog, err := logging.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[log] %v\n", err)
	}
	defer closeLog()

	if len(os.Args) < 2 {
		slog.Error("service started without target executable")
		fmt.Fprintln(os.Stderr, "[service] missing target executable")
		os.Exit(1)
	}

	slog.Info("service startup", "args_count", len(os.Args)-1)

	phantom.Start()
	os.Exit(launch(os.Args[1], os.Args[2:]))
}

// launch spawns the target executable with the tuned JVM flags and
// returns the exit code to propagate to the OS. Nothing sensitive is
// logged — only counts, sizes and redacted paths.
func launch(exePath string, args []string) int {
	sys := sysinfo.Detect()
	slog.Info("system detected",
		"cores", sys.CPUCores,
		"ram_gb", sys.TotalGB(),
		"free_ram_gb", sys.FreeGB(),
		"l3_mb", sys.L3CacheMB,
		"big_cache", sys.HasBigCache(),
		"large_pages", sys.LargePages,
	)

	if err := config.Ensure(sys); err != nil {
		slog.Warn("config ensure failed", "err", err)
		fmt.Fprintf(os.Stderr, "[config] %v\n", err)
	}

	cfg, loadedName, cfgErr := config.LoadActive()
	switch {
	case cfgErr != nil:
		slog.Warn("config load failed, launcher args kept as-is", "err", cfgErr)
	case cfg.HeapSizeGB == 0:
		slog.Warn("config has zero heap, skipping flag injection", "name", loadedName)
	default:
		if requested := config.ActiveName(); requested != "" && requested != loadedName {
			slog.Warn("active config missing, fell back to default",
				"requested", requested,
				"loaded", loadedName,
			)
		}
		flags := jvm.Flags(cfg)
		slog.Info("config loaded",
			"name", loadedName,
			"heap_gb", cfg.HeapSizeGB,
			"metaspace_mb", cfg.MetaspaceMB,
			"parallel_gc", cfg.ParallelGCThreads,
			"conc_gc", cfg.ConcGCThreads,
			"region_mb", cfg.G1HeapRegionSizeMB,
			"pause_ms", cfg.MaxGCPauseMillis,
			"ihop", cfg.InitiatingHeapOccupancyPercent,
			"large_pages", cfg.UseLargePages,
			"flags_count", len(flags),
		)
		args = jvm.FilterArgs(args, flags)
	}

	slog.Info("process starting",
		"exe", logging.RedactPath(exePath),
		"arg_count", len(args),
	)

	proc, err := process.Start(exePath, args)
	if err != nil {
		slog.Error("process start failed", "err", err)
		fmt.Fprintf(os.Stderr, "[process] %v\n", err)
		return 1
	}
	defer proc.Close()
	slog.Info("process started", "pid", proc.PID)

	start := time.Now()
	code, err := proc.Wait()
	waitMs := time.Since(start).Milliseconds()
	if err != nil {
		slog.Error("process wait failed", "err", err, "wait_ms", waitMs)
		fmt.Fprintf(os.Stderr, "[wait] %v\n", err)
		return 1
	}
	slog.Info("service exit", "code", code, "wait_ms", waitMs)
	return code
}
