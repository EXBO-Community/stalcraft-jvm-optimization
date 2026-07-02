// Package logging configures a structured file logger for the wrapper.
//
// Logs land in logs/wrapper.log next to the executable. The package is
// deliberately strict about what can be logged: launcher args, JVM
// flags and environment variables are never written, since they may
// contain session tokens, usernames or auth material. Paths under
// %USERPROFILE% are redacted via RedactPath before being passed to slog.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileName    = "wrapper.log"
	maxFileSize = 2 * 1024 * 1024 // truncate at 2 MB to keep logs bounded
)

// Setup opens the log file and installs it as slog's default handler.
// Returns a close function that should be deferred by the caller.
// If the log file cannot be created the returned close is a no-op and
// slog keeps its pre-existing default — callers should not treat the
// error as fatal.
func Setup() (func(), error) {
	dir := logDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}, fmt.Errorf("create logs dir: %w", err)
	}

	path := filepath.Join(dir, fileName)
	if info, err := os.Stat(path); err == nil && info.Size() > maxFileSize {
		if err := os.Truncate(path, 0); err != nil {
			return func() {}, fmt.Errorf("truncate log: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}, fmt.Errorf("open log: %w", err)
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	return func() { _ = f.Close() }, nil
}

func logDir() string {
	self, err := os.Executable()
	if err != nil {
		return filepath.Join(".", "logs")
	}
	return filepath.Join(filepath.Dir(self), "logs")
}

// RedactPath replaces the Windows user-profile segment in a path with
// the literal "<user>". The rest of the path is preserved so that a
// troubleshooter can see the game directory structure without learning
// who owns the machine.
//
//	C:\Users\Vasya\Games\STALZONE\stalzone.exe
//	  → C:\Users\<user>\Games\STALZONE\stalzone.exe
func RedactPath(p string) string {
	if p == "" {
		return p
	}
	const marker = `\users\`
	lower := strings.ToLower(p)
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return p
	}
	start := idx + len(marker)
	if start >= len(p) {
		return p
	}
	if end := strings.IndexByte(p[start:], '\\'); end >= 0 {
		return p[:start] + "<user>" + p[start+end:]
	}
	return p[:start] + "<user>"
}
