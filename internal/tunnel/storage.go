package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	maxSettingsBytes = 1 << 20
	maxCacheBytes    = 8 << 20
)

func SettingsPath() string {
	return filepath.Join(applicationDir(), "overrides.json")
}

func CachePath() string {
	return filepath.Join(applicationDir(), "cache", "tunnel_stats.json")
}

func LoadSettings() (Settings, error) {
	return loadSettings(SettingsPath())
}

func LoadOverrides() (Settings, error) {
	return loadOverrides(SettingsPath())
}

func SaveSettings(settings Settings) error {
	return saveSettings(SettingsPath(), settings)
}

func LoadCache() (Cache, error) {
	return loadCache(CachePath())
}

func SaveCache(cache Cache) error {
	return saveCache(CachePath(), cache)
}

func loadSettings(path string) (Settings, error) {
	settings := DefaultSettings()
	if err := readJSON(path, &settings, maxSettingsBytes); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return DefaultSettings(), fmt.Errorf("load overrides: %w", err)
	}
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return DefaultSettings(), fmt.Errorf("validate overrides: %w", err)
	}
	return settings, nil
}

func loadOverrides(path string) (Settings, error) {
	settings := DefaultSettings()
	if err := readJSON(path, &settings, maxSettingsBytes); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return DefaultSettings(), fmt.Errorf("load overrides: %w", err)
	}
	settings.Normalize()
	if err := settings.ValidateOverrides(); err != nil {
		return DefaultSettings(), fmt.Errorf("validate overrides: %w", err)
	}
	return settings, nil
}

func saveSettings(path string, settings Settings) error {
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("validate overrides: %w", err)
	}
	if err := writeJSON(path, settings, maxSettingsBytes); err != nil {
		return fmt.Errorf("save overrides: %w", err)
	}
	return nil
}

func loadCache(path string) (Cache, error) {
	cache := NewCache()
	if err := readJSON(path, &cache, maxCacheBytes); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cache, nil
		}
		return NewCache(), fmt.Errorf("load tunnel cache: %w", err)
	}
	cache.Normalize()
	if cache.Version != 1 {
		return NewCache(), fmt.Errorf("unsupported tunnel cache version %d", cache.Version)
	}
	cache.Prune(time.Now())
	return cache, nil
}

func saveCache(path string, cache Cache) error {
	cache.Normalize()
	cache.Prune(time.Now())
	if err := writeJSON(path, cache, maxCacheBytes); err != nil {
		return fmt.Errorf("save tunnel cache: %w", err)
	}
	return nil
}

func readJSON(path string, destination any, maxBytes int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fileOperationError("stat", path, err)
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileOperationError("read", path, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSON(path string, value any, maxBytes int64) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), maxBytes)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

func fileOperationError(operation, path string, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	return fmt.Errorf("%s %s: %w", operation, filepath.Base(path), err)
}

func applicationDir() string {
	executable, err := os.Executable()
	if err == nil {
		return filepath.Dir(executable)
	}
	workingDir, err := os.Getwd()
	if err == nil {
		return workingDir
	}
	return "."
}
