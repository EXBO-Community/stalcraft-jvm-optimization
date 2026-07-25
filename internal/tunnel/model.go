// Package tunnel discovers STALZONE Roxy endpoints, measures their latency,
// and stores the user's optional tunnel override.
package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"
)

const Login = "EXBO-Community"

type Region string

const (
	RegionRU  Region = "ru"
	RegionEU  Region = "eu"
	RegionNA  Region = "na"
	RegionSEA Region = "sea"
	RegionNEA Region = "nea"
)

var regions = []Region{
	RegionRU,
	RegionEU,
	RegionNA,
	RegionSEA,
	RegionNEA,
}

func Regions() []Region {
	return append([]Region(nil), regions...)
}

func (r Region) Valid() bool {
	switch r {
	case RegionRU, RegionEU, RegionNA, RegionSEA, RegionNEA:
		return true
	default:
		return false
	}
}

func (r Region) Label() string {
	return strings.ToUpper(string(r))
}

type SearchMode string

const (
	SearchModePing      SearchMode = "ping"
	SearchModeClientRTT SearchMode = "client_rtt"
	SearchModeServerRTT SearchMode = "server_rtt"
	SearchModeStability SearchMode = "stability"
	SearchModeGame      SearchMode = "game_weighted"
)

var searchModes = []SearchMode{
	SearchModePing,
	SearchModeClientRTT,
	SearchModeServerRTT,
	SearchModeStability,
	SearchModeGame,
}

func SearchModes() []SearchMode {
	return append([]SearchMode(nil), searchModes...)
}

func (m SearchMode) Valid() bool {
	switch m {
	case SearchModePing,
		SearchModeClientRTT,
		SearchModeServerRTT,
		SearchModeStability,
		SearchModeGame:
		return true
	default:
		return false
	}
}

type Endpoint struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Pool struct {
	Name    string     `json:"name"`
	Tunnels []Endpoint `json:"tunnels"`
}

type Catalog struct {
	Mode                    string  `json:"mode"`
	Pools                   []Pool  `json:"pools"`
	ClientToTunnelRTTWeight float64 `json:"clientToTunnelRttWeight"`
}

type Target struct {
	Region   Region
	Pool     string
	Endpoint Endpoint
}

func (t Target) Key() string {
	return CacheKey(t.Region, t.Endpoint.Address)
}

func (c Catalog) Pool(name string) (Pool, bool) {
	for _, pool := range c.Pools {
		if pool.Name == name {
			return pool, true
		}
	}
	return Pool{}, false
}

func (c Catalog) Targets(region Region, excluded func(string) bool) []Target {
	targets := make([]Target, 0)
	seen := make(map[string]struct{})
	for _, pool := range c.Pools {
		if excluded != nil && excluded(pool.Name) {
			continue
		}
		for _, endpoint := range pool.Tunnels {
			if _, ok := seen[endpoint.Address]; ok {
				continue
			}
			seen[endpoint.Address] = struct{}{}
			targets = append(targets, Target{
				Region:   region,
				Pool:     pool.Name,
				Endpoint: endpoint,
			})
		}
	}
	return targets
}

type Selection struct {
	Region  Region `json:"region"`
	Pool    string `json:"pool,omitempty"`
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

func (s Selection) Validate() error {
	if !s.Region.Valid() {
		return fmt.Errorf("invalid tunnel region %q", s.Region)
	}
	if _, _, err := ParseAddress(s.Address); err != nil {
		return fmt.Errorf("invalid tunnel address: %w", err)
	}
	if s.Pool != "" && !validLabel(s.Pool) {
		return fmt.Errorf("invalid tunnel pool %q", s.Pool)
	}
	if s.Name != "" && !validLabel(s.Name) {
		return fmt.Errorf("invalid tunnel name %q", s.Name)
	}
	return nil
}

func (s Selection) JVMFlag() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("-Droxy_address_override.%s=%s", s.Region, s.Address), nil
}

type SearchSettings struct {
	Mode          SearchMode          `json:"mode,omitempty"`
	ExcludedPools map[Region][]string `json:"excluded_pools,omitempty"`
}

type Settings struct {
	Version         int                  `json:"version"`
	TunnelOverrides map[Region]Selection `json:"tunnel_overrides,omitempty"`
	TunnelSearch    SearchSettings       `json:"tunnel_search,omitempty"`

	// TunnelOverride is read only to migrate preview builds that stored a
	// single selection. Normalize moves it into TunnelOverrides before saving.
	TunnelOverride *Selection `json:"tunnel_override,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		Version:         1,
		TunnelOverrides: make(map[Region]Selection),
		TunnelSearch: SearchSettings{
			Mode:          SearchModePing,
			ExcludedPools: make(map[Region][]string),
		},
	}
}

func (s Settings) Clone() Settings {
	clone := s
	clone.TunnelOverrides = make(map[Region]Selection, len(s.TunnelOverrides))
	for region, override := range s.TunnelOverrides {
		clone.TunnelOverrides[region] = override
	}
	if s.TunnelOverride != nil {
		override := *s.TunnelOverride
		clone.TunnelOverride = &override
	}
	clone.TunnelSearch.ExcludedPools = make(map[Region][]string, len(s.TunnelSearch.ExcludedPools))
	for region, pools := range s.TunnelSearch.ExcludedPools {
		clone.TunnelSearch.ExcludedPools[region] = append([]string(nil), pools...)
	}
	clone.Normalize()
	return clone
}

func (s *Settings) Normalize() {
	if s.Version == 0 {
		s.Version = 1
	}
	if s.TunnelOverrides == nil {
		s.TunnelOverrides = make(map[Region]Selection)
	}
	if s.TunnelOverride != nil {
		if _, exists := s.TunnelOverrides[s.TunnelOverride.Region]; !exists {
			s.TunnelOverrides[s.TunnelOverride.Region] = *s.TunnelOverride
		}
		s.TunnelOverride = nil
	}
	if s.TunnelSearch.Mode == "" {
		s.TunnelSearch.Mode = SearchModePing
	}
	if s.TunnelSearch.ExcludedPools == nil {
		s.TunnelSearch.ExcludedPools = make(map[Region][]string)
	}

	for region, pools := range s.TunnelSearch.ExcludedPools {
		seen := make(map[string]struct{}, len(pools))
		normalized := make([]string, 0, len(pools))
		for _, pool := range pools {
			pool = strings.TrimSpace(pool)
			if pool == "" {
				continue
			}
			if _, ok := seen[pool]; ok {
				continue
			}
			seen[pool] = struct{}{}
			normalized = append(normalized, pool)
		}
		if len(normalized) == 0 {
			delete(s.TunnelSearch.ExcludedPools, region)
			continue
		}
		s.TunnelSearch.ExcludedPools[region] = normalized
	}
}

func (s Settings) Validate() error {
	if err := s.ValidateOverrides(); err != nil {
		return err
	}
	if !s.TunnelSearch.Mode.Valid() {
		return fmt.Errorf("invalid tunnel search mode %q", s.TunnelSearch.Mode)
	}
	for region, pools := range s.TunnelSearch.ExcludedPools {
		if !region.Valid() {
			return fmt.Errorf("invalid excluded-pool region %q", region)
		}
		for _, pool := range pools {
			if !validLabel(pool) {
				return fmt.Errorf("invalid excluded tunnel pool %q", pool)
			}
		}
	}
	return nil
}

func (s Settings) ValidateOverrides() error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported overrides version %d", s.Version)
	}
	for region, override := range s.TunnelOverrides {
		if !region.Valid() {
			return fmt.Errorf("invalid tunnel override region %q", region)
		}
		if override.Region != region {
			return fmt.Errorf(
				"tunnel override region %q does not match key %q",
				override.Region,
				region,
			)
		}
		if err := override.Validate(); err != nil {
			return fmt.Errorf("tunnel override %s: %w", region, err)
		}
	}
	return nil
}

func (s Settings) Override(region Region) (Selection, bool) {
	override, ok := s.TunnelOverrides[region]
	return override, ok
}

func (s *Settings) SetOverride(selection Selection) {
	s.Normalize()
	s.TunnelOverrides[selection.Region] = selection
}

func (s *Settings) ClearOverride(region Region) {
	s.Normalize()
	delete(s.TunnelOverrides, region)
}

func (s Settings) JVMFlags() ([]string, error) {
	s.Normalize()
	if err := s.ValidateOverrides(); err != nil {
		return nil, err
	}

	flags := make([]string, 0, len(s.TunnelOverrides))
	for _, region := range Regions() {
		override, ok := s.TunnelOverrides[region]
		if !ok {
			continue
		}
		flag, err := override.JVMFlag()
		if err != nil {
			return nil, fmt.Errorf("%s override: %w", region, err)
		}
		flags = append(flags, flag)
	}
	return flags, nil
}

func (s Settings) IsExcluded(region Region, pool string) bool {
	for _, candidate := range s.TunnelSearch.ExcludedPools[region] {
		if candidate == pool {
			return true
		}
	}
	return false
}

func (s *Settings) ToggleExcluded(region Region, pool string) {
	s.Normalize()
	pools := s.TunnelSearch.ExcludedPools[region]
	for i, candidate := range pools {
		if candidate != pool {
			continue
		}
		pools = append(pools[:i], pools[i+1:]...)
		if len(pools) == 0 {
			delete(s.TunnelSearch.ExcludedPools, region)
		} else {
			s.TunnelSearch.ExcludedPools[region] = pools
		}
		return
	}
	s.TunnelSearch.ExcludedPools[region] = append(pools, pool)
}

func ParseAddress(address string) (host string, port int, err error) {
	if address != strings.TrimSpace(address) {
		return "", 0, fmt.Errorf("address contains surrounding whitespace")
	}
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("split host and port: %w", err)
	}
	if !validHost(host) {
		return "", 0, fmt.Errorf("invalid host %q", host)
	}
	port, err = strconv.Atoi(rawPort)
	if err != nil {
		return "", 0, fmt.Errorf("parse port: %w", err)
	}
	if port < 1 || port >= 65535 {
		return "", 0, fmt.Errorf("port %d cannot use probe port +1", port)
	}
	return host, port, nil
}

func ProbeAddress(address string) (string, error) {
	host, port, err := ParseAddress(address)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port+1)), nil
}

func CacheKey(region Region, address string) string {
	return string(region) + "|" + address
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func validLabel(label string) bool {
	if label == "" || len(label) > 128 || label != strings.TrimSpace(label) {
		return false
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
