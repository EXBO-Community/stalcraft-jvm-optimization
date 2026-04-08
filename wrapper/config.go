package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

type Config struct {
	// Heap
	HeapSizeGB int  `json:"heap_size_gb"`
	PreTouch   bool `json:"pre_touch"`

	// Metaspace
	MetaspaceMB int `json:"metaspace_mb"`

	// G1GC core
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

	// G1 advanced (STW minimization)
	G1SATBBufferEnqueueingThresholdPercent int  `json:"g1_satb_buffer_enqueuing_threshold_percent"`
	G1ConcRSHotCardLimit                   int  `json:"g1_conc_rs_hot_card_limit"`
	G1ConcRefinementServiceIntervalMillis  int  `json:"g1_conc_refinement_service_interval_millis"`
	GCTimeRatio                            int  `json:"gc_time_ratio"`
	UseDynamicNumberOfGCThreads            bool `json:"use_dynamic_number_of_gc_threads"`
	UseStringDeduplication                 bool `json:"use_string_deduplication"`

	// GC threads
	ParallelGCThreads int `json:"parallel_gc_threads"`
	ConcGCThreads     int `json:"conc_gc_threads"`

	// Misc GC
	SoftRefLRUPolicyMSPerMB int `json:"soft_ref_lru_policy_ms_per_mb"`

	// JIT
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

	// Large pages
	UseLargePages bool `json:"use_large_pages"`
}

// wrapperDir returns the directory where the wrapper exe lives.
func wrapperDir() string {
	self, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(self)
}

func configsDir() string {
	return filepath.Join(wrapperDir(), "configs")
}

const registryPath = `Software\StalcraftWrapper`

func ensureConfigExists() {
	dir := configsDir()
	os.MkdirAll(dir, 0755)

	// If no configs at all, generate default
	entries, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(entries) == 0 {
		sys := detectSystem()
		cfg := generateConfig(sys)
		saveConfigAs(cfg, "default")
	}

	// If no active config in registry, set to default
	if getActiveName() == "" {
		setActiveConfig("default")
	}
}

func generateConfig(sys SystemInfo) Config {
	heap := calcHeap(sys)
	parallel, conc := calcGCThreads(sys)
	strong := sys.CPUCores >= 8

	cfg := Config{
		HeapSizeGB:  int(heap),
		PreTouch:    strong,
		MetaspaceMB: calcMetaspace(heap),

		MaxGCPauseMillis:              50,
		G1HeapRegionSizeMB:            calcRegionSize(heap),
		G1NewSizePercent:              23,
		G1MaxNewSizePercent:           40,
		G1ReservePercent:              20,
		G1HeapWastePercent:            5,
		G1MixedGCCountTarget:          4,
		G1MixedGCLiveThresholdPercent: 90,
		SurvivorRatio:                 32,
		MaxTenuringThreshold:          1,

		ParallelGCThreads:      parallel,
		ConcGCThreads:          conc,
		SoftRefLRUPolicyMSPerMB: calcSoftRef(heap),

		MaxInlineLevel: 15,
		FreqInlineSize: 500,

		UseLargePages: sys.LargePages,
	}

	if strong {
		cfg.InitiatingHeapOccupancyPercent = 15
		cfg.G1RSetUpdatingPauseTimePercent = 0
		cfg.G1SATBBufferEnqueueingThresholdPercent = 30
		cfg.G1ConcRSHotCardLimit = 16
		cfg.G1ConcRefinementServiceIntervalMillis = 150
		cfg.GCTimeRatio = 99
		cfg.UseDynamicNumberOfGCThreads = true
		cfg.UseStringDeduplication = true

		cfg.ReservedCodeCacheSizeMB = 400
		cfg.InlineSmallCode = 4000
		cfg.MaxNodeLimit = 240000
		cfg.NodeLimitFudgeFactor = 8000
		cfg.NmethodSweepActivity = 1
		cfg.DontCompileHugeMethods = false
		cfg.AllocatePrefetchStyle = 3
		cfg.AlwaysActAsServerClass = true
		cfg.UseXMMForArrayCopy = true
		cfg.UseFPUForSpilling = true
	} else {
		cfg.InitiatingHeapOccupancyPercent = 30
		cfg.G1RSetUpdatingPauseTimePercent = 5
		cfg.GCTimeRatio = 19

		cfg.ReservedCodeCacheSizeMB = 256
	}

	return cfg
}

func saveConfigAs(cfg Config, name string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := configsDir()
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, name+".json")
	return os.WriteFile(path, data, 0644)
}

func loadActiveConfig() (Config, bool) {
	name := getActiveName()
	if name == "" {
		name = "default"
	}
	path := filepath.Join(configsDir(), name+".json")
	cfgData, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false
	}
	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return Config{}, false
	}
	return cfg, true
}

func setActiveConfig(name string) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	key.SetStringValue("ActiveConfig", name)
}

func listConfigs() []string {
	entries, _ := filepath.Glob(filepath.Join(configsDir(), "*.json"))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		base := filepath.Base(e)
		names = append(names, base[:len(base)-len(".json")])
	}
	return names
}

func getActiveName() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	val, _, err := key.GetStringValue("ActiveConfig")
	if err != nil {
		return ""
	}
	return val
}

func calcHeap(sys SystemInfo) uint64 {
	free := bytesToGB(sys.FreeRAM)
	total := bytesToGB(sys.TotalRAM)

	if total <= 8 || free < 6 {
		return 0
	}

	heap := free / 2
	if heap < 6 {
		heap = 6
	}
	if heap > 8 {
		heap = 8
	}
	return heap
}

func calcGCThreads(sys SystemInfo) (parallel, concurrent int) {
	cores := sys.CPUCores

	// Parallel: only active during STW (app is frozen anyway)
	// Use half the cores — enough to make pauses short
	parallel = cores / 2
	if parallel < 2 {
		parallel = 2
	}

	// Concurrent: runs alongside the game, keep low
	// 1 thread per 4 cores, minimum 1
	concurrent = cores / 4
	if concurrent < 1 {
		concurrent = 1
	}

	return
}

func calcRegionSize(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 8
	case heapGB <= 8:
		return 16
	default:
		return 32
	}
}

func calcMetaspace(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 128
	case heapGB <= 8:
		return 256
	default:
		return 512
	}
}

func calcSoftRef(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 10
	case heapGB <= 8:
		return 25
	default:
		return 50
	}
}
