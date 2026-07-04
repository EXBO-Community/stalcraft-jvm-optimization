# Configuration Parameters

Configuration is fine-grained JVM tuning for your specific hardware. The auto-generated `v1.1.2/default.json` profile covers about 95% of cases: the wrapper inspects your CPU, logical thread count, RAM, memory speed and Large Pages availability, then plugs in values that have been proven to work well on live STALZONE.

This document explains **why** each parameter exists and **which direction** to nudge it when hand-tuning. The exact numbers for your machine are sitting in `configs/v1.1.2/default.json` after first launch — the defaults are already tailored to your hardware.

> **Warning:** a misconfigured parameter often produces results **worse** than "leave it alone". Without a clearly stated problem, keep the auto-generated config. Every manual change should be deliberate.

## Reference material

- [Oracle G1 GC tuning guide](https://docs.oracle.com/javase/8/docs/technotes/guides/vm/gctuning/g1_gc_tuning.html) — authoritative G1 tuning manual
- [JVM/HotSpot flags](https://docs.oracle.com/javase/8/docs/technotes/tools/windows/java.html) — complete list of Java options
- [OpenJDK source](https://github.com/openjdk/jdk) — for when you really want to know
- [JEP index](https://openjdk.org/jeps/) — JVM change history

---

## Memory

### `heap_size_gb`

Fixes the JVM heap size (`-Xmx`) in gigabytes. The heap is where all Java objects live: chunks, entities, texture buffers, world data structures. A bigger heap means rarer GC cycles but longer individual pauses (G1 has to walk more regions). STALZONE's live working set rarely exceeds 4 GB, so growing the heap endlessly is counterproductive.

The wrapper picks between 2 GB on the weakest systems and 6 GB on systems with 16+ GB RAM: 3 GB for 6+ GB RAM, 4 GB for 8+ GB, 5 GB for 12+ GB and 6 GB for 16+ GB. **Manual tuning is almost never needed.** STALZONE usually lives inside a 2-3 GB live set, so heaps above 6 GB tend to increase G1 scan cost more than they help performance.

### `pre_touch`

Forces the JVM to physically commit every heap page at process start instead of lazily allocating them. Without this flag Windows only hands out pages when the game first touches them, and that first touch during gameplay causes a page fault — a 1–5 ms microfreeze.

The generator enables `pre_touch` at **12+ GB RAM**. On weaker systems PreTouch eats too much RAM at launch and does more harm than good (Windows starts paging). The downside is a slightly longer startup because all 4-6 GB of heap get touched up front.

### `metaspace_mb`

Size of the area holding class metadata. STALZONE loads about 11,000 classes (engine + resource packs + lambda-generated classes + reflection accessors), each taking 10-15 KB of metadata, for a peak of 150–220 MB.

**Set 512 MB.** Any less and you risk `OutOfMemoryError: Metaspace` when big resource packs or long sessions (lots of reflection-generated hidden classes) pile up. Any more is just reserved RAM that is never used. The wrapper pins `MetaspaceSize = MaxMetaspaceSize` so the JVM doesn't do periodic expansions (every expansion triggers a full GC — a disaster).

---

## G1 GC — core parameters

### `max_gc_pause_millis`

Target cap on GC pause length. G1 dynamically resizes young gen and picks how many regions to include in mixed GC so pauses stay below this target.

**Lower does not always mean smoother.** The current generator uses 100 ms for normal/unknown memory speed and 150 ms for slow DDR4/DDR3 at 2933 MT/s or below. This looks less aggressive than older 25-35 ms profiles, but in practice it reduces the risk of G1 slicing work into too many pauses that still miss the target. If you manually set 30-50 ms, judge it by frame-time captures and GC logs, not by "smaller number must be better".

### `g1_heap_region_size_mb`

Size of a single G1 region. The heap is partitioned into equal regions, and all of G1's machinery operates on them. Legal values are powers of two from 1 to 32 MB. Smaller regions give G1 finer control over mixed GC (it can pick which ones to collect more precisely), larger regions save on RSet scanning.

**The wrapper sizes this based on heap:** 4 MB for 2-3 GB heap and 8 MB for 4-6 GB heap. Older profiles with 16-32 MB regions were removed: at the current heap sizes, 8 MB gives G1 finer mixed-GC region selection, and STALZONE keeps large mesh/buffer data mostly off-heap.

### `g1_new_size_percent` and `g1_max_new_size_percent`

Minimum and maximum percentage of heap that can be used for young generation (Eden + Survivor). Young gen is where all new objects are born; most die young and a small fraction gets promoted to old gen.

Bigger young gen = fewer young GCs but each one longer. The wrapper uses **30% / 50%** on slow memory and **33% / 50%** on normal/unknown memory. Going below 20% / 40% gives you frequent minor pauses; going above 40% / 60% eats the heap budget old gen needs.

### `g1_reserve_percent`

Fraction of heap G1 holds in reserve for peak allocation spikes between GC cycles. If this reserve runs out you get an emergency full GC, which freezes the game for hundreds of milliseconds.

The generator uses **15%**. This leaves G1 headroom for allocation spikes without taking too much heap away from old gen. If you experiment with a more aggressive young gen, 20% can be tried, but without evacuation/full-GC evidence it is usually unnecessary.

### `g1_heap_waste_percent`

G1's tolerance for "dead" space in old regions. Once more than X% of those regions is garbage, G1 starts mixed GC more aggressively. Lower values = more aggressive G1.

The generator uses **10%**. This is lazier than the older 5% profile and reduces the risk of constant mixed GC on systems where pauses are already memory-bandwidth bound.

### `g1_mixed_gc_count_target`

How many sequential mixed GC cycles G1 spreads old-gen cleanup over. More cycles = shorter each pause, but more pauses overall.

The wrapper uses **4** on slow memory and **6** on normal/unknown memory. More cycles = less work in each mixed GC, but also more pauses and more background CPU pressure.

### `initiating_heap_occupancy_percent`

Heap occupancy at which G1 kicks off a concurrent marking cycle — background scan of old gen to prepare mixed GC. Too high a threshold = concurrent marking can't finish in time, triggering a full GC. Too low = constant background CPU usage.

The generator uses **25%**. This starts concurrent marking before combat bursts and chunk loading can overflow old gen, but without excessive background activity.

### `g1_mixed_gc_live_threshold_percent`

G1 only includes an old region in mixed GC if it has less than X% live objects (the rest is garbage — something worth cleaning). The idea is not to bother with almost-full regions where cleanup yields little.

The generator uses **85%**. This keeps G1 from spending too much time on almost fully-live old regions while still preventing garbage from accumulating unchecked.

### `g1_rset_updating_pause_time_percent`

How much of the GC pause G1 is allowed to spend updating Remembered Sets (the structure tracking cross-region references). Lower = shorter pauses, but part of the work moves to concurrent phase (background CPU load).

The wrapper uses **5%** on slow memory and **8%** on normal/unknown memory. A zero value moves all work into the concurrent phase, but under real gameplay this can create too much background pressure and longer catch-up pauses.

### `survivor_ratio`

Ratio of Eden to each Survivor area inside young gen. Higher values make Survivor smaller and promote surviving objects to old gen faster. Lower values let more objects die in young gen without early promotion.

The generator uses **12**. This gives more Survivor space than the old `32` profile, so short-lived burst objects get a chance to die in young gen instead of being pushed into old gen during combat.

### `max_tenuring_threshold`

Maximum number of young GC cycles an object has to survive before being moved to old gen. With `1`, any object surviving its first young GC immediately goes to old.

The generator uses **3**. This pairs with `survivor_ratio: 12`: objects are not promoted to old gen after the first young GC, but they also do not stay in Survivor for too long.

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

G1 finds identical `String` objects in the background and consolidates their internal char arrays into one shared copy. STALZONE creates tons of duplicate strings (tag names, translation keys, item IDs); dedup saves 100-200 MB of heap over a long session.

The auto-generated profile keeps this disabled (`false`). Deduplication can save heap in a long session, but it adds background work and does not always pay off in a client game. Enable it only as an experiment on CPUs with plenty of spare threads and verify frame time.

---

## GC threads

### `parallel_gc_threads`

Worker count for STW phases (young GC, mixed GC copy phase). These threads only run during pauses, so the count directly affects pause length: more threads = more parallel work = shorter pause.

**Rule: `logical threads - 2`, capped at 10 and floored at 2.** This matters on CPUs without SMT/HT: a 6-core i5-9600KF exposes 6 threads, not 12. The cap of 10 is where G1 usually hits diminishing returns on consumer CPUs.

### `conc_gc_threads`

Worker count for concurrent phases: concurrent marking, concurrent refinement, SATB processing. These threads run alongside the game, stealing CPU cycles.

**Usually `parallel / 2`, minimum 1, maximum 5.** More concurrent workers = faster concurrent phase = lower full GC risk, but they run at the same time as the game. There is no separate X3D branch anymore: testing showed the memory-tier profile to be steadier than special V-Cache numbers.

### `soft_ref_lru_policy_ms_per_mb`

"How many milliseconds the JVM tolerates soft references per MB of free heap". Controls how fast the JVM flushes soft-reference caches (for example LWJGL's texture cache).

**50 is reasonable.** Higher = caches live longer, less GC recreating them, but heap stays occupied. Lower = aggressive flushing, more redundant work. The JVM default is 1000, which is overkill for games with a bounded heap.

---

## JIT compilation

The JVM's JIT compiler translates bytecode into native machine code on the fly. In OpenJDK this is C2 — an aggressive optimizing compiler. Its knobs determine how aggressively the JVM optimizes your code. More aggressive = smoother gameplay, but more memory for the code cache and longer warmup.

### `reserved_code_cache_size_mb`

Maximum size of the compiled JIT code cache. When the cache fills up, JIT stops compiling new methods and starts evicting old ones — catastrophic for FPS stability.

**400 MB is a safe margin.** STALZONE actually uses 150-250 MB, the rest is headroom in case reflection generates lots of compiled accessors. Don't go below 256 MB — you will hit the ceiling.

### `max_inline_level`

Nested inlining depth — how many call levels C2 will unfold into the caller. Deeper inlining = faster hot path, but bigger compiled code.

**15.** The old X3D profile with deeper inlining was removed: live testing showed the regular values to be smoother than the aggressive V-Cache branch.

### `freq_inline_size`

Size threshold for a "hot" method that JIT is allowed to inline despite its size. Normal methods have a stricter size limit for inlining, but frequently-called ones get this larger quota.

**500.** Pairs with `max_inline_level` — together they determine how much code ends up inlined into the hot path.

### `inline_small_code`

Size threshold for a compiled method to be considered "small" and inlined aggressively. Larger value = more methods fall under aggressive inlining.

**4000.** Higher values increase compiled code footprint and are not the default.

### `max_node_limit` and `node_limit_fudge_factor`

`max_node_limit` caps the number of nodes in C2's IR graph for a single method. Complex methods (render loop, chunk mesher) can hit this limit and stay uncompiled — meaning interpretation, i.e. ~10-100x slower. `node_limit_fudge_factor` is the allowance above the limit that C2 may take during optimization.

**240000 / 8000.** These values let C2 compile heavy STALZONE methods. Smaller values (the JVM default is 80000) can leave important methods running in the interpreter, while higher values risk bloating compiled code without practical gain.

### `nmethod_sweep_activity`

Intensity of code cache cleanup for outdated methods. 1 = minimal sweeping, 4 = aggressive.

**1.** STALZONE methods don't go stale after warmup — compiled once, they live until exit. Aggressive sweeping only triggers redundant recompilations.

### `dont_compile_huge_methods`

Forbids JIT compilation of methods over an internal "huge" threshold (~8000 bytecode instructions).

**`false`.** STALZONE has a handful of huge methods (chunk renderer, entity AI) that *must* be compiled. `true` means those stay in the interpreter — constant FPS drops in the relevant scenes.

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

These parameters are specific to OpenJDK 9 (the version STALZONE bundles). On newer Java versions they may behave differently or be absent entirely.

### `reflection_inflation_threshold`

By default the JVM uses a slow interpreted path for the first 15 calls of any reflection method, only then generating a fast bytecode accessor. This is reflection "warmup" and costs startup time.

**0 = compile immediately.** STALZONE heavily uses reflection in its event bus, config loader, mixin loader — the warmup is noticeable. Setting to 0 saves those ~15 calls per method and measurably speeds up startup.

### `auto_box_cache_max`

The JVM caches `Integer.valueOf(n)` objects for the range [-128, 127] — the autobox cache. STALZONE uses `HashMap<Integer, ...>` for block IDs, chunk coords and packet IDs, and those numbers are often outside the default range.

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

## Large Pages

### `use_large_pages`

Enables 2 MB (or 1 GB) memory pages instead of the standard 4 KB. Large pages reduce TLB pressure — the CPU spends fewer cycles on page table walks. For an allocation-heavy workload like STALZONE this can make heap access steadier, but the gain depends on the specific system.

**Requires Windows setup.** Without `SeLockMemoryPrivilege` the JVM silently ignores the flag:

1. `Win + R` → `gpedit.msc`
2. Computer Config → Windows Settings → Security Settings → Local Policies → User Rights Assignment → **Lock pages in memory**
3. Add User → your account
4. **Log out and log back in** (the policy applies at logon)

After that set `use_large_pages: true`. The wrapper checks for the privilege itself when generating the config — if it's missing, this parameter is set to `false` so it doesn't raise false expectations.

On Windows Home (no `gpedit.msc`) large pages cannot be configured out of the box — keep it `false`.
