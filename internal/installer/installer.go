// Package installer wires the wrapper into the game executables via the
// Image File Execution Options "Debugger" key, so Windows launches the
// wrapper whenever the game is started. See Targets for the exact process
// names.
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
// stalzone.exe is used by the standalone launcher, stalzonew.exe by Steam.
var Targets = []string{
	"stalzone.exe",
	"stalzonew.exe",
}

var legacyTargets = []string{"stalcraft.exe", "stalcraftw.exe"}

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

	command := `"` + service + `"`
	changes := make([]debuggerChange, 0, len(Targets))
	for _, target := range Targets {
		change, err := setDebugger(target, command)
		if err != nil {
			slog.Error("installer target failed", "action", "install", "target", target, "err", err)
			if cleanupErr := rollbackChanges(changes, command); cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("rollback partial install: %w", cleanupErr))
			}
			return err
		}
		if change.changed {
			changes = append(changes, change)
		}
		slog.Info("installer target set", "target", target, "debugger", logging.RedactPath(service))
	}
	slog.Info("installer done", "action", "install")
	return nil
}

// ExpectedServicePath returns the service.exe path expected next to the
// currently running binary (normally cli.exe).
func ExpectedServicePath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve self: %w", err)
	}
	return filepath.Join(filepath.Dir(self), serviceName), nil
}

// LocalServiceExists reports whether service.exe exists next to the currently
// running binary. A missing file is reported as exists=false without an error.
func LocalServiceExists() (path string, exists bool, err error) {
	path, err = ExpectedServicePath()
	if err != nil {
		return "", false, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, false, nil
		}
		return path, false, fmt.Errorf("check %s: %w", serviceName, err)
	}
	if info.IsDir() {
		return path, false, fmt.Errorf("%s is a directory, expected executable", path)
	}
	return path, true, nil
}

// resolveService returns the absolute path to service.exe sitting in
// the same directory as the caller (cli.exe). Returns an error with a
// human message if service.exe is missing — this catches the common
// mistake of copying only cli.exe out of the release zip.
func resolveService() (string, error) {
	path, exists, err := LocalServiceExists()
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%s must live next to cli.exe (%s): %w", serviceName, path, os.ErrNotExist)
	}
	return path, nil
}

type debuggerChange struct {
	target       string
	previous     string
	previousType uint32
	changed      bool
}

func setDebugger(target, command string) (debuggerChange, error) {
	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		ifeoPath+`\`+target,
		registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return debuggerChange{}, fmt.Errorf("create IFEO key for %s: %w", target, err)
	}
	defer key.Close()

	previous, previousType, err := key.GetStringValue("Debugger")
	if errors.Is(err, registry.ErrNotExist) {
		previousType = 0
	} else if err != nil {
		return debuggerChange{}, fmt.Errorf("read Debugger for %s; left unchanged: %w", target, err)
	} else if !ownedDebugger(previous) {
		return debuggerChange{}, fmt.Errorf("IFEO Debugger for %s belongs to another program; left unchanged", target)
	}
	if previousType == registry.SZ && previous == command {
		return debuggerChange{}, nil
	}
	if err := key.SetStringValue("Debugger", command); err != nil {
		return debuggerChange{}, fmt.Errorf("set Debugger for %s: %w", target, err)
	}
	return debuggerChange{
		target:       target,
		previous:     previous,
		previousType: previousType,
		changed:      true,
	}, nil
}

func rollbackChanges(changes []debuggerChange, command string) error {
	var errs []error
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if err := restoreDebugger(change, command); err != nil {
			slog.Warn("installer target failed", "action", "rollback", "target", change.target, "err", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func restoreDebugger(change debuggerChange, command string) error {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		ifeoPath+`\`+change.target,
		registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return fmt.Errorf("open IFEO key for %s during rollback: %w", change.target, err)
	}
	defer key.Close()

	current, currentType, err := key.GetStringValue("Debugger")
	if err != nil {
		return fmt.Errorf("read Debugger for %s during rollback: %w", change.target, err)
	}
	if currentType != registry.SZ || current != command {
		return fmt.Errorf("IFEO Debugger for %s changed during install; rollback left it unchanged", change.target)
	}
	switch change.previousType {
	case 0:
		err = key.DeleteValue("Debugger")
	case registry.SZ:
		err = key.SetStringValue("Debugger", change.previous)
	case registry.EXPAND_SZ:
		err = key.SetExpandStringValue("Debugger", change.previous)
	}
	if err != nil {
		return fmt.Errorf("restore Debugger for %s: %w", change.target, err)
	}
	return nil
}

// Uninstall removes only wrapper-owned Debugger values. It preserves each key,
// its permissions, other values and subkeys, and works without service.exe.
// Access-denied errors remain recognizable with errors.Is so callers can
// request elevation for older installations that lack write permission.
func Uninstall() error {
	slog.Info("installer start", "action", "uninstall")
	var errs []error
	for _, targets := range [][]string{Targets, legacyTargets} {
		for _, target := range targets {
			if err := clearDebugger(target); err != nil {
				slog.Warn("installer target failed", "action", "uninstall", "target", target, "err", err)
				errs = append(errs, err)
				continue
			}
			slog.Info("installer target cleared", "action", "uninstall", "target", target)
		}
	}
	slog.Info("installer done", "action", "uninstall", "errors", len(errs))
	return errors.Join(errs...)
}

func clearDebugger(target string) error {
	path := ifeoPath + `\` + target
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read IFEO key for %s: %w", target, err)
	}
	debugger, valueType, err := key.GetStringValue("Debugger")
	key.Close()
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Debugger for %s; left unchanged: %w", target, err)
	}
	if !ownedDebugger(debugger) {
		return fmt.Errorf("IFEO Debugger for %s belongs to another program; left unchanged", target)
	}

	key, err = registry.OpenKey(
		registry.LOCAL_MACHINE,
		path,
		registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open IFEO key for %s to remove Debugger: %w", target, err)
	}
	defer key.Close()
	current, currentType, err := key.GetStringValue("Debugger")
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Debugger for %s before removal: %w", target, err)
	}
	if currentType != valueType || current != debugger {
		return fmt.Errorf("IFEO Debugger for %s changed during uninstall; left unchanged", target)
	}
	if err := key.DeleteValue("Debugger"); err != nil && !errors.Is(err, registry.ErrNotExist) {
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
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target, registry.QUERY_VALUE|registry.WOW64_64KEY)
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
