package config

import "github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"

// Generate produces a performance-oriented Config for the given hardware.
//
// The profile targets a single goal: STALCRAFT running as smoothly as
// possible on a default.json. Values are NOT scaled down to save
// resources — we pick the largest safe number every time.
//
// Only heap size, G1 region size and GC thread count actually depend
// on memory and core count; the JIT/inlining block is scaled by L3
// cache size (X3D-class parts get deeper inlining and a larger node
// budget because their compiled hot path fits entirely in L3).
// Everything else is a fixed, tested default compatible with OpenJDK 9.
func Generate(sys sysinfo.Info) Config {
	heap := sizeHeap(sys.TotalGB())
	parallel, concurrent := gcThreads(sys.CPUThreads)
	jit := jitProfile(sys)

	// Throughput-first defaults for mainstream CPUs. A loose 50 ms
	// pause target lets G1 pick natural-sized young collections
	// instead of slicing them into smaller, more frequent pauses that
	// miss a tighter target anyway. Pair it with a smaller young gen
	// minimum and fewer but larger mixed GC cycles, and aggressive
	// soft-reference cleanup to keep heap pressure down — this is
	// the hand-tuned combo validated on a 9900KF at ~255 FPS stable
	// versus ~233 FPS with the previous latency-biased defaults.
	ihop := 20
	pauseMs := 50
	newSizePercent := 23
	mixedCountTarget := 3
	softRefMs := 25

	if sys.HasBigCache() {
		// X3D-class parts can realistically hit a tight pause budget
		// and benefit from a slightly larger young gen plus longer
		// soft-reference retention for texture caches. Memory bandwidth
		// headroom lets us start concurrent marking earlier without
		// fear of full GC pressure.
		ihop = 15
		pauseMs = 25
		newSizePercent = 30
		mixedCountTarget = 4
		softRefMs = 50
		// Extra concurrent worker only if the OS exposes at least 16
		// logical threads. The naive "cores >= 8" check fires on a
		// 5800X3D / 7800X3D running in "gaming mode" with SMT disabled
		// (8C/8T) and pushes concurrent to 4 — that's 50 % of the CPU
		// taken from the game during marking. Requiring 16+ threads
		// guarantees at least one HT sibling pool to absorb the extra
		// worker without starving the render thread.
		if sys.CPUThreads >= 16 {
			concurrent++
		}
	}

	return Config{
		HeapSizeGB:  int(heap),
		PreTouch:    sys.TotalGB() >= 12,
		MetaspaceMB: 512,

		MaxGCPauseMillis:               pauseMs,
		G1HeapRegionSizeMB:             regionSize(heap),
		G1NewSizePercent:               newSizePercent,
		G1MaxNewSizePercent:            50,
		G1ReservePercent:               20,
		G1HeapWastePercent:             5,
		G1MixedGCCountTarget:           mixedCountTarget,
		InitiatingHeapOccupancyPercent: ihop,
		G1MixedGCLiveThresholdPercent:  90,
		G1RSetUpdatingPauseTimePercent: 0,
		SurvivorRatio:                  32,
		MaxTenuringThreshold:           1,

		G1SATBBufferEnqueueingThresholdPercent: 30,
		G1ConcRSHotCardLimit:                   16,
		G1ConcRefinementServiceIntervalMillis:  150,
		GCTimeRatio:                            99,
		UseDynamicNumberOfGCThreads:            true,
		UseStringDeduplication:                 true,

		ParallelGCThreads:       parallel,
		ConcGCThreads:           concurrent,
		SoftRefLRUPolicyMSPerMB: softRefMs,

		ReservedCodeCacheSizeMB: 400,
		MaxInlineLevel:          jit.maxInlineLevel,
		FreqInlineSize:          jit.freqInlineSize,
		InlineSmallCode:         jit.inlineSmallCode,
		MaxNodeLimit:            jit.maxNodeLimit,
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
		EnableJVMLog:                 true,
	}
}

// jitProfile scales C2 inlining limits with L3 cache. On normal CPUs
// a deeply inlined hot path spills out of L3; on X3D-class parts the
// full compiled working set fits, so deeper inlining is pure win.
type jitLimits struct {
	maxInlineLevel  int
	freqInlineSize  int
	inlineSmallCode int
	maxNodeLimit    int
}

func jitProfile(sys sysinfo.Info) jitLimits {
	if sys.HasBigCache() {
		return jitLimits{
			maxInlineLevel:  20,
			freqInlineSize:  750,
			inlineSmallCode: 6000,
			maxNodeLimit:    320000,
		}
	}
	return jitLimits{
		maxInlineLevel:  15,
		freqInlineSize:  500,
		inlineSmallCode: 4000,
		maxNodeLimit:    240000,
	}
}

// sizeHeap picks a heap size between 2 and 8 GB based on total RAM.
//
// We cap at 8 GB on purpose: STALCRAFT's live working set is ~2-3 GB,
// and larger heaps only inflate G1 scan time without helping throughput.
// The 2 GB floor is the minimum that lets G1 run efficiently; anything
// below and the game runs, but full GCs dominate.
func sizeHeap(totalGB uint64) uint64 {
	switch {
	case totalGB >= 24:
		return 8
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
// A 9900KF benchmark showed that 5 concurrent workers (matching the
// hand-tuned max.json preset) materially outperformed 4 under
// sustained allocation pressure, hence the bump from 4 to 5.
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
// caps heap at 8 GB, so 16 MB is the upper choice in practice —
// 32 MB regions would leave only 256 regions at 8 GB heap, hurting
// mixed-GC selection granularity.
func regionSize(heapGB uint64) int {
	switch {
	case heapGB <= 3:
		return 4
	case heapGB <= 5:
		return 8
	default:
		return 16
	}
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
