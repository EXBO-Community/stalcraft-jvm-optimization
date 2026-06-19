// Package installer wires the wrapper into the game executables via the
// Image File Execution Options "Debugger" key, so Windows launches the
// wrapper whenever the game is started. See Targets for the exact process
// names and the stalcraft.exe -> stalzone.exe rebrand handling.
package installer

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/logging"
)

const (
	ifeoPath    = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
	serviceName = "service.exe"
)

// Targets is the set of game executables rewritten to launch the wrapper.
//
// stalzone.exe / stalzonew.exe are the canonical names after the
// STALCRAFT->STALZONE rebrand and are registered ahead of time: the game
// has not renamed its process binary yet but is expected to. Until then,
// stalcraft.exe / stalcraftw.exe stay the live process names and act as a
// temporary fallback — they become outdated once the rename lands. Hooking
// all four is a harmless superset: an IFEO key for a name that never
// launches simply never fires.
var Targets = []string{
	"stalzone.exe", "stalzonew.exe", // canonical post-rebrand (registered preemptively)
	"stalcraft.exe", "stalcraftw.exe", // current live names, temporary fallback
}

// Entry reports the install state of a single target.
type Entry struct {
	Target    string
	Installed bool
	Debugger  string
}

// Install points the IFEO Debugger for each target at service.exe,
// which must live next to the currently running binary (cli.exe).
// Requires administrator privileges.
func Install() error {
	slog.Info("installer start", "action", "install")

	service, err := resolveService()
	if err != nil {
		slog.Error("installer service lookup failed", "err", err)
		return err
	}

	for _, target := range Targets {
		if err := setDebugger(target, service); err != nil {
			slog.Error("installer target failed", "action", "install", "target", target, "err", err)
			return err
		}
		slog.Info("installer target set", "target", target, "debugger", logging.RedactPath(service))
	}
	slog.Info("installer done", "action", "install")
	return nil
}

// resolveService returns the absolute path to service.exe sitting in
// the same directory as the caller (cli.exe). Returns an error with a
// human message if service.exe is missing — this catches the common
// mistake of copying only cli.exe out of the release zip.
func resolveService() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve self: %w", err)
	}
	path := filepath.Join(filepath.Dir(self), serviceName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%s must live next to cli.exe: %w", serviceName, err)
	}
	return path, nil
}

func setDebugger(target, debugger string) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create IFEO key for %s: %w", target, err)
	}
	defer key.Close()

	if err := key.SetStringValue("Debugger", `"`+debugger+`"`); err != nil {
		return fmt.Errorf("set Debugger for %s: %w", target, err)
	}
	return nil
}

// Uninstall removes the Debugger value for each target, accumulating errors.
func Uninstall() error {
	slog.Info("installer start", "action", "uninstall")
	var errs []error
	for _, target := range Targets {
		if err := clearDebugger(target); err != nil {
			slog.Warn("installer target failed", "action", "uninstall", "target", target, "err", err)
			errs = append(errs, err)
			continue
		}
		slog.Info("installer target cleared", "target", target)
	}
	slog.Info("installer done", "action", "uninstall", "errors", len(errs))
	return errors.Join(errs...)
}

func clearDebugger(target string) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open IFEO key for %s: %w", target, err)
	}
	defer key.Close()

	if err := key.DeleteValue("Debugger"); err != nil {
		return fmt.Errorf("delete Debugger for %s: %w", target, err)
	}
	return nil
}

// Status reads the current Debugger value for each target.
func Status() []Entry {
	entries := make([]Entry, 0, len(Targets))
	for _, target := range Targets {
		entries = append(entries, statusFor(target))
	}
	return entries
}

func statusFor(target string) Entry {
	e := Entry{Target: target}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target, registry.QUERY_VALUE)
	if err != nil {
		return e
	}
	defer key.Close()

	val, _, err := key.GetStringValue("Debugger")
	if err != nil {
		return e
	}
	e.Installed = true
	e.Debugger = val
	return e
}
