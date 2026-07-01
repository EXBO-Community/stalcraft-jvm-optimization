// Package ui renders the interactive arrow-key menu shown when the
// wrapper is launched without CLI arguments.
package ui

import (
	"fmt"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/elevate"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/installer"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

type item struct {
	label  string
	action func() bool // true → pause before clearing screen
}

// Run displays the top-level menu until the user chooses Exit.
func Run() error {
	restoreVT := enableVT()
	defer restoreVT()

	sys := sysinfo.Detect()
	if err := config.Ensure(sys); err != nil {
		return err
	}

	for {
		drawHeader(config.ActiveName(), config.ActiveExists())

		var exit bool
		items := []item{
			{"Install", elevatedAction("--install", "install")},
			{"Uninstall", uninstallAction},
			{"Status", func() bool { PrintStatus(); return true }},
			{"Select Config", func() bool { selectConfig(); return false }},
			{"Regenerate Config", func() bool { regenerate(sys); return true }},
			{"Exit", func() bool { exit = true; return false }},
		}

		wait := runMenu(items)
		if exit {
			return nil
		}
		if wait {
			fmt.Println()
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
		}
		fmt.Print("\033[2J\033[H")
	}
}

func drawHeader(active string, exists bool) {
	fmt.Println("STALZONE JVM Optimization Wrapper")
	fmt.Println("-----------------------------------")
	if active == "" {
		fmt.Println("Active config: (none — default.json will be used)")
	} else if !exists {
		fmt.Printf("Active config: %s  (missing — falls back to default)\n", active)
	} else {
		fmt.Printf("Active config: %s\n", active)
	}
	fmt.Println()
	fmt.Println("RU: Стрелки для выбора, Enter для подтверждения.")
	fmt.Println("EN: Arrow keys to select, Enter to confirm.")
	fmt.Println()
}

func uninstallAction() bool {
	fmt.Println("[uninstall] Removing IFEO hook...")
	if err := installer.Uninstall(); err != nil {
		fmt.Printf("[error] %v\n", err)
		return true
	}
	fmt.Println("[uninstall] Done.")
	return true
}

func elevatedAction(flag, label string) func() bool {
	return func() bool {
		fmt.Printf("[%s] Requesting administrator privileges...\n", label)
		code, err := elevate.Run(flag)
		switch {
		case err != nil:
			fmt.Printf("[error] %v\n", err)
		case code != 0:
			fmt.Printf("[error] %s failed (exit code %d)\n", label, code)
		default:
			fmt.Printf("[%s] Done.\n", label)
		}
		return true
	}
}

// PrintStatus is exposed so main can reuse it for the --status CLI flag.
func PrintStatus() {
	entries := installer.Status()
	anyInstalled := false
	for _, e := range entries {
		if e.Installed {
			fmt.Printf("[status] %s -> %s\n", e.Target, e.Debugger)
			anyInstalled = true
		} else {
			fmt.Printf("[status] %s: not installed\n", e.Target)
		}
	}
	if !anyInstalled {
		fmt.Println("[status] Not installed")
	}
}

func selectConfig() {
	names, err := config.List()
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	if len(names) == 0 {
		fmt.Println("[config] No configs found in configs/")
		return
	}

	active := config.ActiveName()
	items := make([]item, 0, len(names)+1)
	for _, name := range names {
		n := name
		label := "  " + n
		if n == active {
			label = "* " + n
		}
		items = append(items, item{label, func() bool {
			if err := config.SetActive(n); err != nil {
				fmt.Printf("[error] %v\n", err)
				return true
			}
			fmt.Printf("[config] Active config set to: %s\n", n)
			return false
		}})
	}
	items = append(items, item{"< Back", func() bool { return false }})

	fmt.Println()
	fmt.Println("Select config (* = active):")
	runMenu(items)
}

func regenerate(sys sysinfo.Info) {
	fresh := sysinfo.Detect()
	cfg := config.Generate(fresh)

	fmt.Printf("[config] Detected: %s\n", fresh.Describe())
	fmt.Printf("[config] Heap: %d GB, GC threads: %d parallel / %d concurrent\n",
		cfg.HeapSizeGB, cfg.ParallelGCThreads, cfg.ConcGCThreads)

	if fresh.TotalGB() < 8 {
		fmt.Println("[warning] Less than 8 GB RAM — enable the page file to avoid stalls.")
	} else if fresh.TotalGB() <= 16 {
		fmt.Println("[note] 16 GB RAM: page file recommended for comfort.")
	}

	if err := cfg.Save("default"); err != nil {
		fmt.Printf("[error] Failed to save: %v\n", err)
		return
	}
	if err := config.SetActive("default"); err != nil {
		fmt.Printf("[error] Failed to mark active: %v\n", err)
		return
	}
	fmt.Println("[config] Regenerated default config.")
}

func runMenu(items []item) bool {
	restoreCursor := hideCursor()
	defer restoreCursor()
	restoreMode, hIn := rawMode()
	defer restoreMode()

	selected := 0
	drawItems(items, selected)

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
			clearItems(len(items))
			restoreMode()
			return items[selected].action()
		case 0x1B: // VK_ESCAPE
			clearItems(len(items))
			return false
		default:
			continue
		}
		drawItems(items, selected)
	}
}

func drawItems(items []item, selected int) {
	for i := range items {
		fmt.Print("\033[2K\r")
		if i == selected {
			fmt.Printf("  > %s", items[i].label)
		} else {
			fmt.Printf("    %s", items[i].label)
		}
		if i < len(items)-1 {
			fmt.Print("\n")
		}
	}
	fmt.Printf("\033[%dA\r", len(items)-1)
}

func clearItems(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[2K\r")
		if i < n-1 {
			fmt.Print("\n")
		}
	}
	fmt.Printf("\033[%dA\r", n-1)
}
