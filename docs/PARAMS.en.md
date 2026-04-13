# Configuration Parameters

Configuration is fine-grained JVM tuning for your specific hardware. The auto-generated `default.json` covers about 95% of cases: the wrapper inspects your CPU, core count, RAM, and L3 cache size and plugs in values that have been proven to work well on live STALCRAFT.

This document explains **why** each parameter exists and **which direction** to nudge it when hand-tuning. The exact numbers for your machine are sitting in `configs/default.json` after first launch — the defaults are already tailored to your hardware.

> **Warning:** a misconfigured parameter often produces results **worse** than "leave it alone". If you don't have a concrete problem surfaced by `logs/jvm.log`, keep the auto-generated config. Every manual change should be deliberate.

## Reference material

- [Oracle G1 GC tuning guide](https://docs.oracle.com/javase/8/docs/technotes/guides/vm/gctuning/g1_gc_tuning.html) — authoritative G1 tuning manual
- [JVM/HotSpot flags](https://docs.oracle.com/javase/8/docs/technotes/tools/windows/java.html) — complete list of Java options
- [OpenJDK source](https://github.com/openjdk/jdk) — for when you really want to know
- [JEP index](https://openjdk.org/jeps/) — JVM change history

---

## Memory

### `heap_size_gb`

Fixes the JVM heap size (`-Xmx`) in gigabytes. The heap is where all Java objects live: chunks, entities, texture buffers, world data structures. A bigger heap means rarer GC cycles but longer individual pauses (G1 has to walk more regions). STALCRAFT's live working set rarely exceeds 4 GB, so growing the heap endlessly is counterproductive.

The wrapper picks between 2 GB (on 8 GB total RAM) and 8 GB (on 24+ GB total RAM). **Manual tuning is almost never needed.** If you are on 16 GB and STALCRAFT actually reports `OutOfMemoryError` (rare, only with aggressive mod stacks) you can try 6 or 7. Don't go above 8 — that only bloats mixed GC pauses and breaks frame time smoothness.

### `pre_touch`

Forces the JVM to physically commit every heap page at process start instead of lazily allocating them. Without this flag Windows only hands out pages when the game first touches them, and that first touch during gameplay causes a page fault — a 1–5 ms microfreeze.

**Enable (`true`)** only when you have **12+ GB RAM**. On weaker systems PreTouch eats too much RAM at launch and does more harm than good (Windows starts paging). The downside is a 1–3 second longer startup because all 4-8 GB get touched up front.

### `metaspace_mb`

Size of the area holding class metadata. STALCRAFT loads about 11,000 classes (engine + resource packs + lambda-generated classes + reflection accessors), each taking 10-15 KB of metadata, for a peak of 150–220 MB.

**Set 512 MB.** Any less and you risk `OutOfMemoryError: Metaspace` when big resource packs or long sessions (lots of reflection-generated hidden classes) pile up. Any more is just reserved RAM that is never used. The wrapper pins `MetaspaceSize = MaxMetaspaceSize` so the JVM doesn't do periodic expansions (every expansion triggers a full GC — a disaster).

---

## G1 GC — core parameters

### `max_gc_pause_millis`

Target cap on GC pause length. G1 dynamically resizes young gen and picks how many regions to include in mixed GC so pauses stay below this target.

**Lower = smoother frame time, but more GC.** On mainstream hardware 35 ms is a good balance: one missed frame at 60 FPS (16.7 ms frame time — two frames in a row), almost imperceptible to the player. On strong CPUs (X3D-class parts with 96 MB L3) you can push it down to 20-25 ms and get effectively smooth gameplay. On weaker CPUs don't go below 30 ms — G1 simply can't hit the target and will miss it anyway, but overall throughput will suffer.

### `g1_heap_region_size_mb`

Size of a single G1 region. The heap is partitioned into equal regions, and all of G1's machinery operates on them. Legal values are powers of two from 1 to 32 MB. Smaller regions give G1 finer control over mixed GC (it can pick which ones to collect more precisely), larger regions save on RSet scanning.

**The wrapper sizes this based on heap** (4 MB for 2-3 GB heap, 8 MB for 4-5 GB, 16 MB for 6-7 GB, 32 MB for 8 GB). Touch it only if you want more granular mixed GC for a low-latency setup (8 kHz mouse, e-sport tuning) — then step down one size.

### `g1_new_size_percent` and `g1_max_new_size_percent`

Minimum and maximum percentage of heap that can be used for young generation (Eden + Survivor). Young gen is where all new objects are born; most die young and a small fraction gets promoted to old gen.

Bigger young gen = fewer young GCs but each one longer. **Set 30 / 50** — the wrapper does. These values are optimal for STALCRAFT's high allocation rate. Going below 20 / 40 gives you frequent minor pauses; going above 40 / 60 eats the heap budget old gen needs.

### `g1_reserve_percent`

Fraction of heap G1 holds in reserve for peak allocation spikes between GC cycles. If this reserve runs out you get an emergency full GC, which freezes the game for hundreds of milliseconds.

**Set 20%.** Less and you risk full GCs on allocation peaks (particle effects, chunk loading bursts). More and heap usage becomes inefficient. STALCRAFT with its spiky allocation pattern loves headroom.

### `g1_heap_waste_percent`

G1's tolerance for "dead" space in old regions. Once more than X% of those regions is garbage, G1 starts mixed GC more aggressively. Lower values = more aggressive G1.

**5% is a good default.** Aggressive cleanup prevents garbage buildup without being so aggressive that mixed GC becomes constant. You can try 10% if mixed GC pauses bother you, but at the cost of slightly more wasted heap space.

### `g1_mixed_gc_count_target`

How many sequential mixed GC cycles G1 spreads old-gen cleanup over. More cycles = shorter each pause, but more pauses overall.

**4 is the throughput default, 8 is the low-latency choice.** For STALCRAFT leaning on frame time smoothness, 8 is excellent on strong CPUs. On weaker CPUs 4 gives more consistent throughput.

### `initiating_heap_occupancy_percent`

Heap occupancy at which G1 kicks off a concurrent marking cycle — background scan of old gen to prepare mixed GC. Too high a threshold = concurrent marking can't finish in time, triggering a full GC. Too low = constant background CPU usage.

**20% is a safe default, 15% is for strong systems.** On X3D-class parts with big L3 and fast memory you can afford an earlier start — concurrent marking finishes faster and mixed GC kicks in before heap fills up. On weak hardware 20-25% gives concurrent marking more actual time to do its job.

### `g1_mixed_gc_live_threshold_percent`

G1 only includes an old region in mixed GC if it has less than X% live objects (the rest is garbage — something worth cleaning). The idea is not to bother with almost-full regions where cleanup yields little.

**90 is correct.** Low values (65-85) from older guides leave too much garbage in the heap. 90 gives G1 freedom to clean even dense regions when there's still something to reclaim — this prevents long-lived object buildup.

### `g1_rset_updating_pause_time_percent`

How much of the GC pause G1 is allowed to spend updating Remembered Sets (the structure tracking cross-region references). Lower = shorter pauses, but part of the work moves to concurrent phase (background CPU load).

**0% — all concurrent.** On 6+ core CPUs the concurrent refinement threads handle it on their own, no need to do it in STW. On 4-core parts you can set 3-5% to offload background work.

### `survivor_ratio`

Ratio of Eden to each Survivor area inside young gen. At `32` you get Eden = 32 × Survivor, meaning Survivor is tiny. The effect is that objects get promoted to old gen faster (they don't linger in Survivor).

**32 is our "fast promotion" philosophy.** Classic guides recommend 6-8 (big Survivor = objects live longer in young), but for STALCRAFT with its short-lived temporary objects fast promotion wins: less copying between Survivor spaces, less bookkeeping.

### `max_tenuring_threshold`

Maximum number of young GC cycles an object has to survive before being moved to old gen. With `1`, any object surviving its first young GC immediately goes to old.

**1 pairs with `survivor_ratio: 32`.** Together they implement "object either dies very young or goes straight to old gen". Ideal for STALCRAFT: most temporary objects (particle vectors, bounding boxes, transient collections) die in Eden, and the ones that survive a cycle are almost certainly long-lived (entities, chunks) — no point keeping them in young.

---

## G1 GC — advanced STW minimization flags

All parameters below are **experimental** — they need `-XX:+UnlockExperimentalVMOptions`, which the wrapper adds automatically.

### `g1_satb_buffer_enqueuing_threshold_percent`

Threshold at which G1 starts actively draining the SATB (Snapshot-At-The-Beginning) buffer. SATB is how G1 tracks object graph mutations during concurrent marking.

**30 is reasonable.** Draining earlier = less accumulated work, fewer long spike pauses. 0 disables the optimization entirely.

### `g1_conc_rs_hot_card_limit`

G1 marks frequently-updated memory cards as "hot" and handles them separately. This parameter is the threshold where a card becomes hot. Hot cards are processed more often and don't go through the general refinement queue.

**16 is the default.** Works well in most cases. Raise it only if `g1_conc_refinement_service_interval_millis` is using too much CPU.

### `g1_conc_refinement_service_interval_millis`

Interval between background hot-card processing cycles. Lower = more responsive G1, higher = less background CPU load.

**150 ms strikes the balance.** The game doesn't feel the interval, CPU overhead is minimal. Leave it.

### `gc_time_ratio`

Target ratio of application time to GC time. 99 means "1 minute of GC is OK for every 99 minutes of app runtime" = 1% overhead. G1 uses this for adaptive decisions.

**99 is the standard.** No need to touch, just leave it.

### `use_dynamic_number_of_gc_threads`

Lets G1 adjust the number of active GC workers based on load. Useful on modern CPUs with P+E cores (Intel 12+), where G1 may migrate between cores of different performance.

**Enable (`true`)** everywhere except the weakest CPUs. On 4-core parts the savings aren't worth the added latency variance.

### `use_string_deduplication`

G1 finds identical `String` objects in the background and consolidates their internal char arrays into one shared copy. STALCRAFT creates tons of duplicate strings (tag names, translation keys, item IDs); dedup saves 100-200 MB of heap over a long session.

**Enable (`true`)** on 8+ core CPUs. On weak CPUs the ~1% CPU overhead from the dedup thread may be noticeable, but on strong parts this is pure heap savings.

---

## GC threads

### `parallel_gc_threads`

Worker count for STW phases (young GC, mixed GC copy phase). These threads only run during pauses, so the count directly affects pause length: more threads = more parallel work = shorter pause.

**Rule: `physical cores - 2`, capped at 10 and floored at 2.** Leave 2 cores for the main game thread and render thread. The cap of 10 is where G1 hits diminishing returns on consumer CPUs. The wrapper computes this automatically.

### `conc_gc_threads`

Worker count for concurrent phases: concurrent marking, concurrent refinement, SATB processing. These threads run alongside the game, stealing CPU cycles.

**Usually `parallel / 4`, minimum 1, maximum 4.** More concurrent workers = faster concurrent phase = lower full GC risk, but they steal cores from the game. On X3D-class parts the wrapper adds +1 to the default — strong cores can afford concurrent work without hurting FPS.

### `soft_ref_lru_policy_ms_per_mb`

"How many milliseconds the JVM tolerates soft references per MB of free heap". Controls how fast the JVM flushes soft-reference caches (for example LWJGL's texture cache).

**50 is reasonable.** Higher = caches live longer, less GC recreating them, but heap stays occupied. Lower = aggressive flushing, more redundant work. The JVM default is 1000, which is overkill for games with a bounded heap.

---

## JIT compilation

The JVM's JIT compiler translates bytecode into native machine code on the fly. In OpenJDK this is C2 — an aggressive optimizing compiler. Its knobs determine how aggressively the JVM optimizes your code. More aggressive = smoother gameplay, but more memory for the code cache and longer warmup.

### `reserved_code_cache_size_mb`

Maximum size of the compiled JIT code cache. When the cache fills up, JIT stops compiling new methods and starts evicting old ones — catastrophic for FPS stability.

**400 MB is a safe margin.** STALCRAFT actually uses 150-250 MB, the rest is headroom in case reflection generates lots of compiled accessors. Don't go below 256 MB — you will hit the ceiling.

### `max_inline_level`

Nested inlining depth — how many call levels C2 will unfold into the caller. Deeper inlining = faster hot path, but bigger compiled code.

**15 on mainstream CPUs, 20 on X3D-class parts with large L3.** With 96 MB of L3 cache you can afford aggressive inlining — the hot path fits entirely in cache. On regular CPUs with 16-32 MB L3, deep inlining evicts hot data from cache: you win in one place, lose in another.

### `freq_inline_size`

Size threshold for a "hot" method that JIT is allowed to inline despite its size. Normal methods have a stricter size limit for inlining, but frequently-called ones get this larger quota.

**500 on mainstream, 750 on big-cache.** Pairs with `max_inline_level` — together they determine how much code ends up inlined into the hot path.

### `inline_small_code`

Size threshold for a compiled method to be considered "small" and inlined aggressively. Larger value = more methods fall under aggressive inlining.

**4000 for normal, 6000 for big-cache.** Same pattern — more CPU cache = more compiled code can live inline.

### `max_node_limit` and `node_limit_fudge_factor`

`max_node_limit` caps the number of nodes in C2's IR graph for a single method. Complex methods (render loop, chunk mesher) can hit this limit and stay uncompiled — meaning interpretation, i.e. ~10-100x slower. `node_limit_fudge_factor` is the allowance above the limit that C2 may take during optimization.

**240000 / 8000 for normal CPUs, 320000 / 8000 for big-cache.** These values let C2 compile even STALCRAFT's heaviest methods. Smaller values (the JVM default is 80000) leave several important methods running in the interpreter.

### `nmethod_sweep_activity`

Intensity of code cache cleanup for outdated methods. 1 = minimal sweeping, 4 = aggressive.

**1.** STALCRAFT methods don't go stale after warmup — compiled once, they live until exit. Aggressive sweeping only triggers redundant recompilations.

### `dont_compile_huge_methods`

Forbids JIT compilation of methods over an internal "huge" threshold (~8000 bytecode instructions).

**`false`.** STALCRAFT has a handful of huge methods (chunk renderer, entity AI) that *must* be compiled. `true` means those stay in the interpreter — constant FPS drops in the relevant scenes.

### `allocate_prefetch_style`

Software prefetch strategy during new-object allocation. C2 emits `prefetch` instructions before TLAB allocations to pull memory into cache lines early.

**3 = maximally aggressive.** On modern CPUs the prefetch instruction cost is essentially zero, while the effect on allocation-heavy workloads is noticeable. 0 disables prefetch entirely — don't.

### `always_act_as_server_class`

Forces the JVM to always use the server JIT (C2) for top-tier compilation instead of client JIT (C1). On Windows the JVM detects "server-class" hardware automatically and sometimes gets it wrong.

**`true`.** Guarantees C2 kicks in even on atypical configs. Increases warmup time, but that's a one-time cost for a long gameplay session.

### `use_xmm_for_array_copy`

Use XMM (SSE2) registers for array copies. These SIMD instructions copy 16 bytes per cycle instead of 8.

**`true`** on every CPU newer than Pentium 4. A pure win for any copy operation (String.clone, Array.copy, LWJGL buffer memory ops).

### `use_fpu_for_spilling`

Allows C2 to use FPU/SSE registers for spilling values when general-purpose registers run out. An alternative to saving values on the stack.

**`true`.** Spilling to FPU is faster than to the stack (no memory access), which stabilizes frame time in register-heavy scenes.

---

## Java 9 specifics

These parameters are specific to OpenJDK 9 (the version STALCRAFT bundles). On newer Java versions they may behave differently or be absent entirely.

### `reflection_inflation_threshold`

By default the JVM uses a slow interpreted path for the first 15 calls of any reflection method, only then generating a fast bytecode accessor. This is reflection "warmup" and costs startup time.

**0 = compile immediately.** STALCRAFT heavily uses reflection in its event bus, config loader, mixin loader — the warmup is noticeable. Setting to 0 saves those ~15 calls per method and measurably speeds up startup.

### `auto_box_cache_max`

The JVM caches `Integer.valueOf(n)` objects for the range [-128, 127] — the autobox cache. STALCRAFT uses `HashMap<Integer, ...>` for block IDs, chunk coords and packet IDs, and those numbers are often outside the default range.

**4096.** Extends the cache to [-128, 4095] — now all block IDs (about 2000 in vanilla + mods) fall inside, and each `Integer.valueOf(blockId)` stops creating a new object. This removes millions of allocations from the renderer and network hot paths.

### `use_thread_priorities` and `thread_priority_policy`

Let the JVM translate Java's `Thread.setPriority()` into real Windows thread priorities. By default the Windows JVM clamps everything to `NORMAL`, ignoring setPriority calls entirely.

**`true` + policy `1`** unlock the full priority range. The LWJGL render thread and main game loop get a higher priority than GC workers, which gives steadier frame time. Policy 1 = "aggressive" — uses every Windows priority level, including above NORMAL.

### `use_counter_decay`

Roughly every 10 seconds the JVM decays JIT hotness counters (periodic counter decay). The idea is that "formerly hot" methods should yield their spot to newly hot ones — but in a game every hot method (render, AI, physics) is hot the entire session.

**`false` = disable decay.** Counters accumulate monotonically, hot methods stay compiled forever, no recompilations from a metric "cooling down".

### `compile_threshold_scaling`

Multiplier on the C1→C2 promotion threshold. 1.0 is the default (~10,000 invocations before C2), 0.5 is twice as early (~5,000).

**0.5 = faster warmup.** Methods hit their final C2 version sooner, the game reaches peak performance faster after loading. The downside is a tiny bit more CPU during warmup (first minute of play), but this is a one-off price for steadier gameplay.

---

## Logging

### `enable_jvm_log`

Enables [JDK 9 unified logging](https://openjdk.org/jeps/158) for GC, safepoints, JIT compilation, deoptimization, metaspace and code cache events. Output lands in `logs/jvm.log` inside the game directory with 3 × 10 MB rotation (up to 30 MB on disk total).

**`true` = on.** Overhead is below 0.1% (unified logging is non-blocking), and in exchange you can diagnose virtually any JVM-related freeze: `grep "stopped:" logs/jvm.log | awk '$NF > 0.015'` surfaces every STW pause longer than 15 ms with its reason. Without this log, the root cause of microfreezes is a guessing game.

Turn off (`false`) only if your disk is extremely constrained and 30 MB matters.

---

## Large Pages

### `use_large_pages`

Enables 2 MB (or 1 GB) memory pages instead of the standard 4 KB. Large pages reduce TLB pressure — the CPU spends fewer cycles on page table walks. For an allocation-heavy workload like STALCRAFT the win is 2-5% throughput plus reduced TLB jitter (critical for latency-sensitive setups like 8 kHz mice).

**Requires Windows setup.** Without `SeLockMemoryPrivilege` the JVM silently ignores the flag:

1. `Win + R` → `gpedit.msc`
2. Computer Config → Windows Settings → Security Settings → Local Policies → User Rights Assignment → **Lock pages in memory**
3. Add User → your account
4. **Log out and log back in** (the policy applies at logon)

After that set `use_large_pages: true`. The wrapper checks for the privilege itself when generating the config — if it's missing, this parameter is set to `false` so it doesn't raise false expectations.

On Windows Home (no `gpedit.msc`) large pages cannot be configured out of the box — keep it `false`.
