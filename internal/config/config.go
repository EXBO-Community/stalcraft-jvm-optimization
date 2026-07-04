// Package config models JVM tuning profiles and stores them as JSON files
// under configs/. The package also owns the "active" pointer in HKCU.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const registryPath = `Software\StalcraftWrapper`

var (
	// ErrNotFound is returned when a config file does not exist on disk.
	ErrNotFound = errors.New("config not found")
	// ErrInvalidName is returned when a config id cannot be represented as a
	// relative path below configs/.
	ErrInvalidName = errors.New("invalid config name")
)

type Config struct {
	HeapSizeGB  int  `json:"heap_size_gb"`
	PreTouch    bool `json:"pre_touch"`
	MetaspaceMB int  `json:"metaspace_mb"`

	MaxGCPauseMillis               int `json:"max_gc_pause_millis"`
	G1HeapRegionSizeMB             int `json:"g1_heap_region_size_mb"`
	G1NewSizePercent               int `json:"g1_new_size_percent"`
	G1MaxNewSizePercent            int `json:"g1_max_new_size_percent"`
	G1ReservePercent               int `json:"g1_reserve_percent"`
	G1HeapWastePercent             int `json:"g1_heap_waste_percent"`
	G1MixedGCCountTarget           int `json:"g1_mixed_gc_count_target"`
	InitiatingHeapOccupancyPercent int `json:"initiating_heap_occupancy_percent"`
	G1MixedGCLiveThresholdPercent  int `json:"g1_mixed_gc_live_threshold_percent"`
	G1RSetUpdatingPauseTimePercent int `json:"g1_rset_updating_pause_time_percent"`
	SurvivorRatio                  int `json:"survivor_ratio"`
	MaxTenuringThreshold           int `json:"max_tenuring_threshold"`

	G1SATBBufferEnqueueingThresholdPercent int  `json:"g1_satb_buffer_enqueuing_threshold_percent"`
	G1ConcRSHotCardLimit                   int  `json:"g1_conc_rs_hot_card_limit"`
	G1ConcRefinementServiceIntervalMillis  int  `json:"g1_conc_refinement_service_interval_millis"`
	GCTimeRatio                            int  `json:"gc_time_ratio"`
	UseDynamicNumberOfGCThreads            bool `json:"use_dynamic_number_of_gc_threads"`
	UseStringDeduplication                 bool `json:"use_string_deduplication"`

	ParallelGCThreads int `json:"parallel_gc_threads"`
	ConcGCThreads     int `json:"conc_gc_threads"`

	SoftRefLRUPolicyMSPerMB int `json:"soft_ref_lru_policy_ms_per_mb"`

	ReservedCodeCacheSizeMB int  `json:"reserved_code_cache_size_mb"`
	MaxInlineLevel          int  `json:"max_inline_level"`
	FreqInlineSize          int  `json:"freq_inline_size"`
	InlineSmallCode         int  `json:"inline_small_code"`
	MaxNodeLimit            int  `json:"max_node_limit"`
	NodeLimitFudgeFactor    int  `json:"node_limit_fudge_factor"`
	NmethodSweepActivity    int  `json:"nmethod_sweep_activity"`
	DontCompileHugeMethods  bool `json:"dont_compile_huge_methods"`
	AllocatePrefetchStyle   int  `json:"allocate_prefetch_style"`
	AlwaysActAsServerClass  bool `json:"always_act_as_server_class"`
	UseXMMForArrayCopy      bool `json:"use_xmm_for_array_copy"`
	UseFPUForSpilling       bool `json:"use_fpu_for_spilling"`

	UseLargePages bool `json:"use_large_pages"`

	// Java 9-era flags: reflection fast path, integer boxing cache,
	// thread scheduling, JIT counter retention, and a C1->C2 promotion
	// multiplier. Loop Strip Mining is intentionally absent:
	// -XX:LoopStripMiningIter was only added in JDK 10.
	ReflectionInflationThreshold int     `json:"reflection_inflation_threshold"`
	AutoBoxCacheMax              int     `json:"auto_box_cache_max"`
	UseThreadPriorities          bool    `json:"use_thread_priorities"`
	ThreadPriorityPolicy         int     `json:"thread_priority_policy"`
	UseCounterDecay              bool    `json:"use_counter_decay"`
	CompileThresholdScaling      float64 `json:"compile_threshold_scaling"`
}

// Entry is a child item inside configs/<prefix>/.
type Entry struct {
	Name  string
	ID    string
	IsDir bool
}

// Dir returns the configs directory next to the executable.
// Falls back to ./configs if the executable path can't be resolved.
func Dir() string {
	self, err := os.Executable()
	if err != nil {
		return filepath.Join(".", "configs")
	}
	return filepath.Join(filepath.Dir(self), "configs")
}

// EnsureDir creates configs/ next to the executable.
func EnsureDir() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return fmt.Errorf("create configs dir: %w", err)
	}
	return nil
}

// Save writes the config to configs/<name>.json. The name may be nested,
// e.g. "v1.1.2/default".
func (c Config) Save(name string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path, err := filePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config parent: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Load reads configs/<name>.json. The name may be nested, e.g.
// "v1.1.2/default".
func Load(name string) (Config, error) {
	path, err := filePath(name)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Exists reports whether configs/<name>.json exists.
func Exists(name string) bool {
	path, err := filePath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// LoadActive reads the currently selected config. If the active config is
// unset or missing, fallback is loaded instead.
func LoadActive(fallback string) (cfg Config, loadedName string, err error) {
	if fallback == "" {
		fallback = "default"
	}
	requested := ActiveName()
	if requested == "" {
		requested = fallback
	}
	cfg, err = Load(requested)
	if errors.Is(err, ErrNotFound) && requested != fallback {
		if fallbackCfg, fallbackErr := Load(fallback); fallbackErr == nil {
			return fallbackCfg, fallback, nil
		}
	}
	return cfg, requested, err
}

// ActiveExists reports whether the selected active config exists on disk.
func ActiveExists() bool {
	name := ActiveName()
	return name != "" && Exists(name)
}

// List returns every config id under configs/.
func List() ([]string, error) {
	dir := Dir()
	names := make([]string, 0)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		names = append(names, name)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan configs: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

// ListDir returns direct child directories and JSON configs under
// configs/<prefix>/.
func ListDir(prefix string) ([]Entry, error) {
	dir := Dir()
	if prefix != "" {
		rel, err := cleanName(prefix)
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(dir, filepath.FromSlash(rel))
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan configs dir: %w", err)
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			out = append(out, Entry{Name: name, ID: joinName(prefix, name), IsDir: true})
			continue
		}
		if filepath.Ext(name) != ".json" {
			continue
		}
		base := strings.TrimSuffix(name, ".json")
		out = append(out, Entry{Name: base, ID: joinName(prefix, base)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// SetActive records the active config name in HKCU.
func SetActive(name string) error {
	clean, err := cleanName(name)
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue("ActiveConfig", clean); err != nil {
		return fmt.Errorf("set ActiveConfig: %w", err)
	}
	return nil
}

// ActiveName reads the active config name from HKCU, empty string if unset.
func ActiveName() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	val, _, err := key.GetStringValue("ActiveConfig")
	if err != nil {
		return ""
	}
	clean, err := cleanName(val)
	if err != nil {
		return ""
	}
	return clean
}

func filePath(name string) (string, error) {
	rel, err := cleanName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(Dir(), filepath.FromSlash(rel)+".json"), nil
}

func cleanName(name string) (string, error) {
	name = filepath.ToSlash(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidName)
	}
	if strings.HasSuffix(name, ".json") {
		name = strings.TrimSuffix(name, ".json")
	}
	if strings.ContainsAny(name, `:*?"<>|`) {
		return "", fmt.Errorf("%w: %s", ErrInvalidName, name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("%w: %s", ErrInvalidName, name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: %s", ErrInvalidName, name)
		}
	}
	return clean, nil
}

func joinName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}
