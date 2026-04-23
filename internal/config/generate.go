package config

import "github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"

// Generate produces a performance-oriented Config for the given hardware.
//
// The profile targets a single goal: STALCRAFT running as smoothly as
// possible on a default.json. Values are NOT scaled down to save
// resources — we pick the largest safe number every time.
//
// Only heap size, G1 region size and GC thread count actually depend
// on memory and core count; everything else is a fixed, tested default
// compatible with OpenJDK 9. The tier-specific pause/mixed-count block
// below is keyed on DDR memory speed (slow/mid/fast).
//
// X3D-specific tuning was attempted (halved pause budget, boosted soft-
// ref retention, clamped concurrent worker count, deeper JIT inlining)
// but community testing on a 9800X3D + DDR5-6200 rig showed the non-
// X3D mid-tier profile outperforming it on perceived smoothness. The
// X3D branch was removed entirely; V-Cache parts are treated as
// regular fast-tier hardware driven only by MemTier().
func Generate(sys sysinfo.Info) Config {
	heap := sizeHeap(sys.TotalGB())
	parallel, concurrent := gcThreads(sys.CPUThreads)

	// Memory-bandwidth-aware frame-pacing profile. Young-GC copy cost
	// is bandwidth-bound, so the realistic pause target and the
	// granularity of mixed-GC work scale with configured memory speed.
	//
	//   slow (≤ 2933 MT/s)    — stutters >50 ms dominated by fixed RSet
	//                           scan cost per mixed-GC pass. Fewer,
	//                           longer passes (mixedCount=4) amortise
	//                           the scan overhead; a looser pause
	//                           target (150 ms) stops G1 from slicing
	//                           young collections into more pauses
	//                           than the memory can actually complete.
	//   mid (everything else) — pauseMs=100, mixedCount=6, rsetUpd=8,
	//                           newSize=33. Covers XMP-enabled DDR4
	//                           and all DDR5. An earlier fast tier for
	//                           DDR5 ≥ 4800 MT/s (pauseMs=80 /
	//                           mixedCount=8) caused periodic freezes
	//                           on a 9800X3D + DDR5-6200 rig while
	//                           this exact mid-tier profile ran
	//                           smoothly; fast tier was removed.
	//
	// Other pauses-sensitive flags (survivor sizing, tenuring, IHOP,
	// live-region threshold, soft-ref retention) are not bandwidth-
	// dependent and stay common across tiers.
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
	default: // MemMid and unknown
		pauseMs = 100
		mixedCountTarget = 6
		rsetUpdatingPct = 8
		newSizePercent = 33
	}

	// Combat-biased baseline: STALCRAFT is effectively always in combat
	// (projectile events, hit registration, particle bursts, AI ticks).
	// An earlier ihop of 35 and tenuring of 6 optimised for idle /
	// animation-heavy states and left combat bursts to spill into the
	// old gen mid-fight, producing the 1-second stalls a 5700X +
	// DDR4-3600 tester reported. Lower IHOP starts concurrent marking
	// before the burst fills old gen; tenuring=3 lets short-lived combat
	// objects die in survivor before being force-promoted; a larger
	// survivor (ratio 12 instead of the Oracle default 8) absorbs the
	// burst without overflowing into old gen in the first place.
	ihop := 25
	softRefMs := 50
	tenuring := 3
	survivorRatio := 12

	return Config{
		HeapSizeGB:  int(heap),
		PreTouch:    sys.TotalGB() >= 12,
		MetaspaceMB: 512,

		MaxGCPauseMillis:               pauseMs,
		G1HeapRegionSizeMB:             regionSize(heap),
		G1NewSizePercent:               newSizePercent,
		G1MaxNewSizePercent:            50,
		G1ReservePercent:               15,
		G1HeapWastePercent:             10,
		G1MixedGCCountTarget:           mixedCountTarget,
		InitiatingHeapOccupancyPercent: ihop,
		G1MixedGCLiveThresholdPercent:  85,
		G1RSetUpdatingPauseTimePercent: rsetUpdatingPct,
		SurvivorRatio:                  survivorRatio,
		MaxTenuringThreshold:           tenuring,

		G1SATBBufferEnqueueingThresholdPercent: 30,
		G1ConcRSHotCardLimit:                   16,
		G1ConcRefinementServiceIntervalMillis:  150,
		GCTimeRatio:                            99,
		UseDynamicNumberOfGCThreads:            true,
		UseStringDeduplication:                 false,

		ParallelGCThreads:       parallel,
		ConcGCThreads:           concurrent,
		SoftRefLRUPolicyMSPerMB: softRefMs,

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

// sizeHeap picks a heap size between 2 and 6 GB based on total RAM.
//
// We cap at 6 GB: STALCRAFT's live working set is ~2-3 GB, larger
// heaps only inflate G1 scan time without helping throughput, and
// since Xms == Xmx (full pre-commit + pre-touch) every extra GB is
// paid for at startup regardless of whether it ever gets used. An
// earlier revision capped at 8 GB with Xms=4 as a compromise; after
// switching to full pre-commit, the 8 GB tier became pure waste on
// 32 GB rigs and was removed. The 2 GB floor is the minimum that
// lets G1 run efficiently; below that full GCs dominate.
func sizeHeap(totalGB uint64) uint64 {
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

// gcThreads derives the STW and concurrent GC worker counts from the
// total logical thread count reported by the OS (runtime.NumCPU).
//
// Parallel workers only run during STW — the game thread is stopped
// anyway, so HT/SMT siblings are free to do GC work without any
// contention. We scale parallel as "threads − 2" (leaving two threads
// to the OS and background services even during STW) and cap at 10
// where G1 hits diminishing returns on consumer hardware.
//
// Concurrent workers share CPU with the running game, so they stay
// a bit more conservative: roughly half of parallel, floor 1, ceiling 5.
// An earlier revision clamped this tighter on X3D parts (2-3) under
// the hypothesis that V-Cache contention with concurrent marking hurt
// render-thread smoothness. Community testing on a 9800X3D + DDR5-6200
// disproved it: five concurrent workers finish the mark cycle faster,
// shortening the total window of mutator/GC overlap; the clamp made
// things worse. Treat X3D parts identically to non-X3D.
//
// Using logical threads (runtime.NumCPU) instead of physical_cores×2
// is essential for correctness on CPUs without SMT/HT: an Intel
// i5-9600KF is 6C/6T, not 6C/12T, and feeding 10 parallel workers to
// a 6-thread CPU oversubscribes it by 1.67× — context switching
// overhead wipes out the throughput gain from extra workers.
func gcThreads(threads int) (parallel, concurrent int) {
	parallel = clamp(threads-2, 2, 10)
	concurrent = clamp(parallel/2, 1, 5)
	return
}

// regionSize matches G1 region granularity to heap size. JVM only
// accepts powers of two between 1 and 32 MB; larger regions mean fewer
// RSet scans, smaller regions mean finer mixed-GC control. sizeHeap
// caps heap at 8 GB, and CapFrameX measurements on both an X3D with
// 8 GB heap and an i5-10400F with 5 GB heap showed 8 MB regions
// outperforming 16 MB — more regions gives mixed-GC selection finer
// granularity so each pass evacuates a smaller, more focused set.
// Stalcraft's large mesh data lives in LWJGL direct buffers off-heap,
// so the 4 MB humongous threshold at 8 MB regions is not a concern.
func regionSize(heapGB uint64) int {
	if heapGB <= 3 {
		return 4
	}
	return 8
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
