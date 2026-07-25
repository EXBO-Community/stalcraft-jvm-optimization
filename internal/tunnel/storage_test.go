package tunnel

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSettingsStorageRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	settings := DefaultSettings()
	settings.TunnelSearch.Mode = SearchModeStability
	settings.ToggleExcluded(RegionRU, "MSK2")
	settings.SetOverride(Selection{
		Region:  RegionRU,
		Pool:    "MSK1",
		Name:    "MSK1-1",
		Address: "192.0.2.10:29450",
	})
	settings.SetOverride(Selection{
		Region:  RegionEU,
		Pool:    "FRA",
		Name:    "FRA-1",
		Address: "192.0.2.20:29450",
	})

	if err := saveSettings(path, settings); err != nil {
		t.Fatalf("saveSettings(): %v", err)
	}
	got, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings(): %v", err)
	}
	for _, region := range []Region{RegionRU, RegionEU} {
		override, ok := got.Override(region)
		want, wantOK := settings.Override(region)
		if !ok || !wantOK || override != want {
			t.Fatalf("%s override = %#v, want %#v", region, override, want)
		}
	}
	if got.TunnelSearch.Mode != SearchModeStability ||
		!got.IsExcluded(RegionRU, "MSK2") {
		t.Fatalf("search settings were not preserved: %#v", got.TunnelSearch)
	}
}

func TestMissingSettingsUsesDefaults(t *testing.T) {
	t.Parallel()

	got, err := loadSettings(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("loadSettings(): %v", err)
	}
	if got.TunnelSearch.Mode != SearchModePing || len(got.TunnelOverrides) != 0 {
		t.Fatalf("defaults = %#v", got)
	}
	if got.Version != 1 {
		t.Fatalf("default version = %d, want 1", got.Version)
	}
}

func TestLegacySingleOverrideIsMigrated(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	legacy := `{
		"version": 1,
		"tunnel_override": {
			"region": "ru",
			"pool": "MSK1",
			"name": "MSK1-1",
			"address": "192.0.2.10:29450"
		},
		"tunnel_search": {"mode": "ping"}
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	settings, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings(): %v", err)
	}
	override, ok := settings.Override(RegionRU)
	if !ok || override.Address != "192.0.2.10:29450" {
		t.Fatalf("migrated override = %#v, found %t", override, ok)
	}
	if settings.TunnelOverride != nil {
		t.Fatal("legacy override was not cleared after migration")
	}

	if err := saveSettings(path, settings); err != nil {
		t.Fatalf("saveSettings(): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if bytes.Contains(data, []byte(`"tunnel_override"`)) {
		t.Fatalf("saved settings retained legacy field:\n%s", data)
	}
	if !bytes.Contains(data, []byte(`"tunnel_overrides"`)) {
		t.Fatalf("saved settings are missing multi-region field:\n%s", data)
	}
}

func TestInvalidSettingsAreRejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"tunnel_search":{"mode":"unknown"}}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if _, err := loadSettings(path); err == nil {
		t.Fatal("loadSettings() accepted an invalid search mode")
	}
}

func TestLoadOverridesIgnoresInvalidSearchMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	data := `{
		"version": 1,
		"tunnel_overrides": {
			"ru": {
				"region": "ru",
				"address": "192.0.2.10:29450"
			}
		},
		"tunnel_search": {
			"mode": "future-mode",
			"excluded_pools": {"unknown": ["bad pool"]}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	settings, err := loadOverrides(path)
	if err != nil {
		t.Fatalf("loadOverrides(): %v", err)
	}
	flags, err := settings.JVMFlags()
	if err != nil {
		t.Fatalf("JVMFlags(): %v", err)
	}
	if len(flags) != 1 ||
		flags[0] != "-Droxy_address_override.ru=192.0.2.10:29450" {
		t.Fatalf("JVMFlags() = %#v", flags)
	}
}

func TestLoadOverridesRejectsInvalidOverride(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	data := `{
		"version": 1,
		"tunnel_overrides": {
			"ru": {
				"region": "ru",
				"address": "192.0.2.10:65535"
			}
		},
		"tunnel_search": {
			"mode": "future-mode"
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	if _, err := loadOverrides(path); err == nil {
		t.Fatal("loadOverrides() accepted an invalid tunnel override")
	}
}

func TestUnsupportedSettingsVersionIsRejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "overrides.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"version":2,"tunnel_search":{"mode":"ping"}}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if _, err := loadSettings(path); err == nil {
		t.Fatal("loadSettings() accepted an unsupported version")
	}
}

func TestCacheStoragePreservesLastLimitState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cache", "tunnel_stats.json")
	now := time.Now().UTC().Truncate(time.Second)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Record(
		target,
		ProbeResult{
			ClientRTT:    15 * time.Millisecond,
			ServerRTT:    3 * time.Millisecond,
			LimitReached: true,
		},
		nil,
		now,
	)

	if err := saveCache(path, cache); err != nil {
		t.Fatalf("saveCache(): %v", err)
	}
	got, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache(): %v", err)
	}
	history, ok := got.History(target.Region, target.Endpoint.Address)
	if !ok {
		t.Fatal("cache history is missing")
	}
	if !history.IsLastTimeLimitReached || !history.LastLimitCheckedAt.Equal(now) {
		t.Fatalf("last limit state was not preserved: %#v", history)
	}
}

func TestWriteJSONRejectsPayloadOverLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "oversized.json")
	err := writeJSON(path, strings.Repeat("x", 128), 32)
	if err == nil {
		t.Fatal("writeJSON() accepted an oversized payload")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("oversized destination exists: %v", statErr)
	}
}

func TestReadJSONDoesNotExposeParentPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	var destination map[string]any
	err := readJSON(path, &destination, maxSettingsBytes)
	if err == nil {
		t.Fatal("readJSON() returned no error")
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("readJSON() error exposes parent path: %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readJSON() error = %v, want os.ErrNotExist", err)
	}
}
