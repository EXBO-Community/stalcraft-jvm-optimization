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
	"unsafe"

	"golang.org/x/sys/windows"
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

	usersSID, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		return fmt.Errorf("resolve BUILTIN\\Users SID: %w", err)
	}

	configured := make([]string, 0, len(Targets))
	for _, target := range Targets {
		modified, err := setDebugger(target, service, usersSID)
		if err != nil {
			slog.Error("installer target failed", "action", "install", "target", target, "err", err)
			rollbackTargets := append([]string(nil), configured...)
			if modified {
				rollbackTargets = append(rollbackTargets, target)
			}
			cleanupErr := deleteTargets("rollback", rollbackTargets)
			if cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("rollback partial install: %w", cleanupErr))
			}
			return err
		}
		configured = append(configured, target)
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

func setDebugger(target, debugger string, deleteSID *windows.SID) (modified bool, err error) {
	key, openedExisting, err := registry.CreateKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target, registry.ALL_ACCESS)
	if err != nil {
		return false, fmt.Errorf("create IFEO key for %s: %w", target, err)
	}
	defer key.Close()
	modified = !openedExisting

	if err := key.SetStringValue("Debugger", `"`+debugger+`"`); err != nil {
		return modified, fmt.Errorf("set Debugger for %s: %w", target, err)
	}
	modified = true
	if err := allowKeyDelete(key, deleteSID); err != nil {
		return modified, fmt.Errorf("grant delete permission for %s: %w", target, err)
	}
	return modified, nil
}

func allowKeyDelete(key registry.Key, sid *windows.SID) error {
	sd, err := windows.GetSecurityInfo(
		windows.Handle(key),
		windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return nil
	}
	if daclAllowsDelete(dacl, sid) {
		return nil
	}

	ace := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.DELETE,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
	newDACL, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{ace}, dacl)
	if err != nil {
		return err
	}

	return windows.SetSecurityInfo(
		windows.Handle(key),
		windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		newDACL,
		nil,
	)
}

func daclAllowsDelete(dacl *windows.ACL, sid *windows.SID) bool {
	if dacl == nil {
		return false
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if ace.Mask&windows.DELETE == 0 {
			continue
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if windows.EqualSid(aceSID, sid) {
			return true
		}
	}
	return false
}

// Uninstall removes each IFEO target key, accumulating errors.
func Uninstall() error {
	slog.Info("installer start", "action", "uninstall")
	errs := deleteTargets("uninstall", Targets)
	slog.Info("installer done", "action", "uninstall", "errors", countErrors(errs))
	return errs
}

func deleteTargets(action string, targets []string) error {
	var errs []error
	for _, target := range targets {
		if err := deleteTargetKey(target); err != nil {
			slog.Warn("installer target failed", "action", action, "target", target, "err", err)
			errs = append(errs, err)
			continue
		}
		slog.Info("installer target cleared", "action", action, "target", target)
	}
	return errors.Join(errs...)
}

func deleteTargetKey(target string) error {
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, ifeoPath+`\`+target); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete IFEO key for %s: %w", target, err)
	}
	return nil
}

func countErrors(err error) int {
	if err == nil {
		return 0
	}
	type unwrapper interface {
		Unwrap() []error
	}
	if joined, ok := err.(unwrapper); ok {
		return len(joined.Unwrap())
	}
	return 1
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
