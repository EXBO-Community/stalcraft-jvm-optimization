package profile

import (
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

func generateV104Default(sys sysinfo.Info) config.Config {
	heap := legacyHeapV104(sys)
	parallel, concurrent := legacyThreadsV104(sys.CPUThreads)
	survivorRatio, tenuring := legacySurvivor(sys.CPUThreads)

	return legacyConfigBase(sys, heap, parallel, concurrent, config.Config{
		PreTouch:                               true,
		MaxGCPauseMillis:                       50,
		G1HeapRegionSizeMB:                     legacyRegionV104(heap),
		G1NewSizePercent:                       30,
		G1MaxNewSizePercent:                    40,
		G1ReservePercent:                       15,
		G1HeapWastePercent:                     5,
		G1MixedGCCountTarget:                   4,
		InitiatingHeapOccupancyPercent:         35,
		G1MixedGCLiveThresholdPercent:          90,
		G1RSetUpdatingPauseTimePercent:         5,
		SurvivorRatio:                          survivorRatio,
		MaxTenuringThreshold:                   tenuring,
		SoftRefLRUPolicyMSPerMB:                legacySoftRef(heap),
		ReservedCodeCacheSizeMB:                legacyCodeCache(heap),
		MaxInlineLevel:                         15,
		FreqInlineSize:                         500,
		DontCompileHugeMethods:                 true,
		ReflectionInflationThreshold:           15,
		UseCounterDecay:                        true,
		G1SATBBufferEnqueueingThresholdPercent: 0,
	})
}

func generateV105Default(sys sysinfo.Info) config.Config {
	return generateV104Default(sys)
}

func generateV106Default(sys sysinfo.Info) config.Config {
	heap := legacyHeapV106(sys)
	parallel, concurrent := legacyThreadsV104(sys.CPUThreads)
	survivorRatio, tenuring := legacySurvivor(sys.CPUThreads)

	return legacyConfigBase(sys, heap, parallel, concurrent, config.Config{
		PreTouch:                       true,
		MaxGCPauseMillis:               50,
		G1HeapRegionSizeMB:             legacyRegionV104(heap),
		G1NewSizePercent:               30,
		G1MaxNewSizePercent:            40,
		G1ReservePercent:               15,
		G1HeapWastePercent:             5,
		G1MixedGCCountTarget:           4,
		InitiatingHeapOccupancyPercent: 35,
		G1MixedGCLiveThresholdPercent:  90,
		G1RSetUpdatingPauseTimePercent: 5,
		SurvivorRatio:                  survivorRatio,
		MaxTenuringThreshold:           tenuring,
		SoftRefLRUPolicyMSPerMB:        legacySoftRef(heap),
		ReservedCodeCacheSizeMB:        legacyCodeCache(heap),
		MaxInlineLevel:                 15,
		FreqInlineSize:                 500,
		DontCompileHugeMethods:         true,
		ReflectionInflationThreshold:   15,
		UseCounterDecay:                true,
	})
}

func generateV107Default(sys sysinfo.Info) config.Config {
	return generateV107Like(sys, 6, false)
}

func generateV108Default(sys sysinfo.Info) config.Config {
	return generateV107Like(sys, 4, true)
}

func generateV107Like(sys sysinfo.Info, minHeap uint64, dontCompileHugeStandard bool) config.Config {
	heap := legacyHeapV107(sys, minHeap)
	parallel, concurrent := legacyThreadsV107(sys.CPUCores)
	strong := sys.CPUCores >= 8

	cfg := legacyConfigBase(sys, heap, parallel, concurrent, config.Config{
		PreTouch:                      strong,
		MaxGCPauseMillis:              50,
		G1HeapRegionSizeMB:            legacyRegionV107(heap),
		G1NewSizePercent:              23,
		G1MaxNewSizePercent:           40,
		G1ReservePercent:              20,
		G1HeapWastePercent:            5,
		G1MixedGCCountTarget:          4,
		G1MixedGCLiveThresholdPercent: 90,
		SurvivorRatio:                 32,
		MaxTenuringThreshold:          1,
		SoftRefLRUPolicyMSPerMB:       legacySoftRef(heap),
		MaxInlineLevel:                15,
		FreqInlineSize:                500,
		ReflectionInflationThreshold:  15,
		UseCounterDecay:               true,
	})

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
		return cfg
	}

	cfg.InitiatingHeapOccupancyPercent = 30
	cfg.G1RSetUpdatingPauseTimePercent = 5
	cfg.GCTimeRatio = 19
	cfg.ReservedCodeCacheSizeMB = 256
	cfg.DontCompileHugeMethods = dontCompileHugeStandard
	return cfg
}

func legacyConfigBase(sys sysinfo.Info, heap uint64, parallel, concurrent int, cfg config.Config) config.Config {
	cfg.HeapSizeGB = int(heap)
	cfg.MetaspaceMB = legacyMetaspace(heap)
	cfg.ParallelGCThreads = parallel
	cfg.ConcGCThreads = concurrent
	cfg.UseLargePages = sys.LargePages
	return cfg
}

func legacyHeapV104(sys sysinfo.Info) uint64 {
	free := sys.FreeGB()
	total := sys.TotalGB()
	if total <= 8 {
		return 0
	}

	heap := free / 2
	floor := total / 4
	if floor < 6 {
		floor = 6
	}
	cap := total * 3 / 4
	if cap > 16 {
		cap = 16
	}
	if heap < floor {
		heap = floor
	}
	if heap > cap {
		heap = cap
	}
	if heap < 6 {
		heap = 6
	}
	return heap
}

func legacyHeapV106(sys sysinfo.Info) uint64 {
	free := sys.FreeGB()
	total := sys.TotalGB()
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

func legacyHeapV107(sys sysinfo.Info, minHeap uint64) uint64 {
	free := sys.FreeGB()
	total := sys.TotalGB()
	if total <= 8 || free < 6 {
		return 0
	}
	heap := free / 2
	if heap < minHeap {
		heap = minHeap
	}
	if heap > 8 {
		heap = 8
	}
	return heap
}

func legacyThreadsV104(threads int) (parallel, concurrent int) {
	parallel = threads - 2
	if parallel < 2 {
		parallel = 2
	}
	concurrent = parallel / 4
	if concurrent < 1 {
		concurrent = 1
	}
	return
}

func legacyThreadsV107(cores int) (parallel, concurrent int) {
	parallel = cores / 2
	if parallel < 2 {
		parallel = 2
	}
	concurrent = cores / 4
	if concurrent < 1 {
		concurrent = 1
	}
	return
}

func legacyRegionV104(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 4
	case heapGB <= 8:
		return 8
	case heapGB <= 16:
		return 16
	default:
		return 32
	}
}

func legacyRegionV107(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 8
	case heapGB <= 8:
		return 16
	default:
		return 32
	}
}

func legacyMetaspace(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 128
	case heapGB <= 8:
		return 256
	default:
		return 512
	}
}

func legacyCodeCache(heapGB uint64) int {
	cc := int(heapGB * 1024 / 16)
	if cc < 128 {
		return 128
	}
	if cc > 512 {
		return 512
	}
	return cc
}

func legacySurvivor(threads int) (ratio, tenuring int) {
	if threads <= 4 {
		return 32, 1
	}
	return 8, 4
}

func legacySoftRef(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 10
	case heapGB <= 8:
		return 25
	default:
		return 50
	}
}
