package tunnel

import (
	"slices"
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		host    string
		port    int
		wantErr bool
	}{
		{name: "IPv4", address: "127.0.0.1:29450", host: "127.0.0.1", port: 29450},
		{name: "IPv6", address: "[2001:db8::1]:29450", host: "2001:db8::1", port: 29450},
		{name: "DNS", address: "tunnel.example.com:29450", host: "tunnel.example.com", port: 29450},
		{name: "missing port", address: "tunnel.example.com", wantErr: true},
		{name: "surrounding whitespace", address: " 127.0.0.1:29450", wantErr: true},
		{name: "invalid host", address: "bad_host:29450", wantErr: true},
		{name: "probe port overflow", address: "127.0.0.1:65535", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, port, err := ParseAddress(tt.address)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAddress(%q) returned no error", tt.address)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tt.address, err)
			}
			if host != tt.host || port != tt.port {
				t.Fatalf(
					"ParseAddress(%q) = (%q, %d), want (%q, %d)",
					tt.address,
					host,
					port,
					tt.host,
					tt.port,
				)
			}
		})
	}
}

func TestProbeAddressAddsOneToPort(t *testing.T) {
	t.Parallel()

	got, err := ProbeAddress("127.0.0.1:29450")
	if err != nil {
		t.Fatalf("ProbeAddress(): %v", err)
	}
	if got != "127.0.0.1:29451" {
		t.Fatalf("ProbeAddress() = %q, want %q", got, "127.0.0.1:29451")
	}
}

func TestSelectionJVMFlag(t *testing.T) {
	t.Parallel()

	selection := Selection{
		Region:  RegionRU,
		Pool:    "MSK2",
		Name:    "MSK2-1",
		Address: "192.0.2.15:29450",
	}
	got, err := selection.JVMFlag()
	if err != nil {
		t.Fatalf("JVMFlag(): %v", err)
	}
	const want = "-Droxy_address_override.ru=192.0.2.15:29450"
	if got != want {
		t.Fatalf("JVMFlag() = %q, want %q", got, want)
	}
}

func TestSettingsJVMFlagsIncludesEverySelectedRegion(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings()
	settings.SetOverride(Selection{
		Region:  RegionEU,
		Address: "192.0.2.20:29450",
	})
	settings.SetOverride(Selection{
		Region:  RegionRU,
		Address: "192.0.2.10:29450",
	})

	got, err := settings.JVMFlags()
	if err != nil {
		t.Fatalf("JVMFlags(): %v", err)
	}
	want := []string{
		"-Droxy_address_override.ru=192.0.2.10:29450",
		"-Droxy_address_override.eu=192.0.2.20:29450",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("JVMFlags() = %#v, want %#v", got, want)
	}
}

func TestCatalogTargetsExcludesPoolsAndDuplicateAddresses(t *testing.T) {
	t.Parallel()

	catalog := Catalog{
		Pools: []Pool{
			{
				Name: "MSK1",
				Tunnels: []Endpoint{
					{Name: "one", Address: "192.0.2.10:29450"},
				},
			},
			{
				Name: "MSK2",
				Tunnels: []Endpoint{
					{Name: "duplicate", Address: "192.0.2.10:29450"},
					{Name: "two", Address: "192.0.2.11:29450"},
				},
			},
			{
				Name: "MSK3",
				Tunnels: []Endpoint{
					{Name: "excluded", Address: "192.0.2.12:29450"},
				},
			},
		},
	}

	targets := catalog.Targets(RegionRU, func(pool string) bool {
		return pool == "MSK3"
	})
	if len(targets) != 2 {
		t.Fatalf("Targets() returned %d targets, want 2: %#v", len(targets), targets)
	}
	if targets[0].Pool != "MSK1" || targets[1].Endpoint.Name != "two" {
		t.Fatalf("Targets() = %#v", targets)
	}
}

func TestSettingsNormalizeAndToggleExclusion(t *testing.T) {
	t.Parallel()

	settings := Settings{
		TunnelSearch: SearchSettings{
			ExcludedPools: map[Region][]string{
				RegionRU: {" MSK2 ", "MSK2", ""},
			},
		},
	}
	settings.Normalize()

	if settings.TunnelSearch.Mode != SearchModePing {
		t.Fatalf("default mode = %q, want %q", settings.TunnelSearch.Mode, SearchModePing)
	}
	if got := settings.TunnelSearch.ExcludedPools[RegionRU]; len(got) != 1 || got[0] != "MSK2" {
		t.Fatalf("normalized exclusions = %#v, want [MSK2]", got)
	}

	settings.ToggleExcluded(RegionRU, "MSK2")
	if settings.IsExcluded(RegionRU, "MSK2") {
		t.Fatal("MSK2 remained excluded after toggle")
	}
	settings.ToggleExcluded(RegionRU, "MSK2")
	if !settings.IsExcluded(RegionRU, "MSK2") {
		t.Fatal("MSK2 was not excluded after second toggle")
	}
}

func TestSettingsCloneDoesNotShareMutableState(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings()
	settings.ToggleExcluded(RegionRU, "MSK2")
	settings.SetOverride(Selection{
		Region:  RegionRU,
		Address: "192.0.2.10:29450",
	})

	clone := settings.Clone()
	clone.ToggleExcluded(RegionRU, "MSK1")
	override, ok := clone.Override(RegionRU)
	if !ok {
		t.Fatal("clone is missing RU override")
	}
	override.Address = "192.0.2.11:29450"
	clone.SetOverride(override)

	if settings.IsExcluded(RegionRU, "MSK1") {
		t.Fatal("clone shares exclusions with original")
	}
	original, ok := settings.Override(RegionRU)
	if !ok || original.Address != "192.0.2.10:29450" {
		t.Fatal("clone shares override with original")
	}
}

func TestSettingsRejectsInvalidStoredValues(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings()
	settings.TunnelOverrides[RegionRU] = Selection{
		Region:  Region("unknown"),
		Address: "127.0.0.1:29450",
	}
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Validate() error = %v, want mismatched region", err)
	}
}

func TestJVMFlagsIgnoreUnrelatedSearchSettings(t *testing.T) {
	t.Parallel()

	settings := DefaultSettings()
	settings.TunnelSearch.Mode = SearchMode("future-mode")
	settings.TunnelSearch.ExcludedPools[Region("future-region")] = []string{"bad\npool"}
	settings.SetOverride(Selection{
		Region:  RegionRU,
		Address: "192.0.2.10:29450",
	})

	got, err := settings.JVMFlags()
	if err != nil {
		t.Fatalf("JVMFlags(): %v", err)
	}
	want := []string{"-Droxy_address_override.ru=192.0.2.10:29450"}
	if !slices.Equal(got, want) {
		t.Fatalf("JVMFlags() = %#v, want %#v", got, want)
	}
}
