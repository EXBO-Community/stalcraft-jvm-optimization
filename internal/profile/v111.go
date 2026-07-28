package profile

import (
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

func generateV111Default(sys sysinfo.Info) config.Config {
	heap := sizeHeapV111(sys.TotalGB())
	parallel, concurrent := gcThreadsModern(sys.CPUThreads)

	var (
		pauseMs          int
		mixedCountTarget int
		rsetUpdatingPct  int
		newSizePercent   int
	)
	switch sys.MemTier() {
	case sysinfo.MemSlow:
		pauseMs = 150
		mixedCountTarget = 4
		rsetUpdatingPct = 5
		newSizePercent = 30
	default:
		pauseMs = 100
		mixedCountTarget = 6
		rsetUpdatingPct = 8
		newSizePercent = 33
	}

	return config.Config{
		HeapSizeGB:  int(heap),
		PreTouch:    sys.TotalGB() >= 12,
		MetaspaceMB: 512,

		MaxGCPauseMillis:               pauseMs,
		G1HeapRegionSizeMB:             regionSizeV111(heap),
		G1NewSizePercent:               newSizePercent,
		G1MaxNewSizePercent:            50,
		G1ReservePercent:               15,
		G1HeapWastePercent:             10,
		G1MixedGCCountTarget:           mixedCountTarget,
		InitiatingHeapOccupancyPercent: 25,
		G1MixedGCLiveThresholdPercent:  85,
		G1RSetUpdatingPauseTimePercent: rsetUpdatingPct,
		SurvivorRatio:                  12,
		MaxTenuringThreshold:           3,

		G1SATBBufferEnqueueingThresholdPercent: 30,
		G1ConcRSHotCardLimit:                   16,
		G1ConcRefinementServiceIntervalMillis:  150,
		GCTimeRatio:                            99,
		UseDynamicNumberOfGCThreads:            true,
		UseStringDeduplication:                 false,

		ParallelGCThreads:       parallel,
		ConcGCThreads:           concurrent,
		SoftRefLRUPolicyMSPerMB: 50,

		ReservedCodeCacheSizeMB: 400,
		MaxInlineLevel:          15,
		FreqInlineSize:          500,
		InlineSmallCode:         4000,
		MaxNodeLimit:            240000,
		NodeLimitFudgeFactor:    8000,
		NmethodSweepActivity:    1,
		DontCompileHugeMethods:  false,
		AllocatePrefetchStyle:   3,
		AlwaysActAsServerClass:  true,
		UseXMMForArrayCopy:      true,
		UseFPUForSpilling:       true,

		UseLargePages: sys.LargePages,

		ReflectionInflationThreshold: 0,
		AutoBoxCacheMax:              4096,
		UseThreadPriorities:          true,
		ThreadPriorityPolicy:         1,
		UseCounterDecay:              false,
		CompileThresholdScaling:      0.5,
	}
}

func generateV112Default(sys sysinfo.Info) config.Config {
	return generateV111Default(sys)
}

func sizeHeapV111(totalGB uint64) uint64 {
	switch {
	case totalGB >= 16:
		return 6
	case totalGB >= 12:
		return 5
	case totalGB >= 8:
		return 4
	case totalGB >= 6:
		return 3
	default:
		return 2
	}
}

func regionSizeV111(heapGB uint64) int {
	if heapGB <= 3 {
		return 4
	}
	return 8
}

func gcThreadsModern(threads int) (parallel, concurrent int) {
	parallel = clamp(threads-2, 2, 10)
	concurrent = clamp(parallel/2, 1, 5)
	return
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
