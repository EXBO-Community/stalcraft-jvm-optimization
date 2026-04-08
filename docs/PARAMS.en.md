# Configuration Parameters

Before manually configuring the JVM, it is important to understand that this section is advisory in nature.
The values provided in the tables are averaged and do not account for the specifics of your particular system.
Each parameter change should be made consciously - you need to understand its purpose and possible consequences.
Otherwise, it is better not to perform manual configuration and use the ready-made profiles or default.json,
automatically generated for your system by the utility.

Additionally, familiarize yourself with Oracle resources:

- [G1 GC tuning](https://docs.oracle.com/javase/8/docs/technotes/guides/vm/gctuning/g1_gc_tuning.html)
- [List of JVM/HotSpot options](https://docs.oracle.com/javase/8/docs/technotes/tools/windows/java.html)
- [General JVM options guide](https://www.oracle.com/java/technologies/javase/vmoptions-jsp.html)

Studying the OpenJDK source code will give a complete understanding of what is happening:

- [OpenJDK source](https://github.com/openjdk/jdk)
- [JEP docs](https://openjdk.org/jeps/)

## Memory

| Parameter        | Description                                                                   | Type       | Recommendation                                                                                                                                   |
| ---------------- | ----------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `heap_size_gb`   | Sets the fixed size of JVM heap memory (Xmx/Xms) in gigabytes.                | `int`      | ~8-12 GB with 16+ GB free RAM. Don't forget about background RAM consumption by other processes.                                                 |
| `pre_touch`      | Forces the JVM to pre-allocate all heap memory when the process starts.       | `boolean`  | Set to `true`. Reduces random microfreezes when allocating memory, but increases game startup time.                                              |
| `metaspace_mb`   | Sets the initial size of Metaspace.                                           | `int`      | ~256–512 MB (sometimes up to 768 MB), to avoid frequent Metaspace expansions and associated microfreezes. Increase together with `heap_size_gb`. |
  
## G1GC — Basic

| Parameter                              | Description                                                                                                | Type   | Recommendation                                                                                                                                 |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `max_gc_pause_millis`                  | Sets the target maximum pause of the garbage collector in milliseconds                                     | `int`  | ~20-50. Lower values make GC more aggressive and level FPS, higher values reduce CPU load but increase risk of noticeable freezes.             |
| `g1_heap_region_size_mb`               | Sets the size of one heap region in G1 GC, from which memory management is built.                          | `int`  | Do not change, leave auto. When manually tuning, consider your heap size. Can be set to ~4-16.                                                 |
| `g1_new_size_percent`                  | Sets the minimum fraction of heap allocated for young generation in G1 GC.                                 | `int`  | ~20%, reduces risk of long GC pauses.                                                                                                          |
| `g1_max_new_size_percent`              | Sets the maximum fraction of heap that young generation can occupy in G1 GC.                               | `int`  | ~40%, gives GC flexibility under load.                                                                                                         |
| `g1_reserve_percent`                   | Sets the fraction of heap reserved by G1 GC to prevent memory shortage under peak loads.                   | `int`  | ~15-20%, provides good headroom for peak allocations.                                                                                          |
| `g1_heap_waste_percent`                | Sets the allowed percentage of unused heap space, after which G1 GC starts more aggressive region cleanup. | `int`  | ~5%-10%, achieves balance between smoothness and load.                                                                                         |
| `g1_mixed_gc_count_target`             | Sets the target number of mixed GC cycles in G1 GC.                                                        | `int`  | ~2-4, evens out frame time.                                                                                                                    |
| `initiating_heap_occupancy_percent`    | Sets the heap fill threshold at which G1 GC starts concurrent marking.                                     | `int`  | ~25%-35%. Lower values may give choppy frame time, higher values may give freezes.                                                             |
| `g1_mixed_gc_live_threshold_percent`   | Include region in mixed GC only if it contains < X% live objects                                           | `int`  | ~65%-85%, to avoid major and prolonged freezes.                                                                                                |
| `g1_rset_updating_pause_time_percent`  | Sets the fraction of GC pause time that G1 can spend updating `rs_hot`.                                    | `int`  | ~5%-10%. Lower values make pauses smoother.<br>Higher values reduce GC overhead but may lead to more noticeable peak stops.                    |
| `survivor_ratio`                       | Sets the ratio between Eden and Survivor regions in young generation.                                      | `int`  | 6-8. Reduces pressure on minor GC and lowers risk of microfreezes.                                                                             |
| `max_tenuring_threshold`               | Sets the maximum number of young generation collections after which an object moves to old gen.            | `int`  | ~4-8. Balance between frequent moves and keeping objects in young gen.<br>Can try 1.                                                           |

## G1GC — Advanced (STW Minimization)

| Parameter                                     | Description                                                                                                       | Type       | Recommendation                                                                                                                                                                       |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `g1_satb_buffer_enqueuing_threshold_percent`  | Sets the SATB buffer fill threshold at which G1 GC starts processing it more actively.                            | `int`      | ~30-60, slightly increase if needed to reduce background GC load.<br>0 - Disabled.                                                                                                   |
| `g1_conc_rs_hot_card_limit`                   | Sets the threshold for "hot" memory cards, at which G1 GC more actively processes changes in region references.   | `int`      | ~16-32 can be slightly increased to reduce background GC activity.<br>0 - Disabled.                                                                                                  |
| `g1_conc_refinement_service_interval_millis`  | Sets the interval in milliseconds between cycles of background `rs_hot` processing.                               | `int`      | ~10–20, lowering values makes GC more responsive but increases background CPU load.<br>0 - Disabled                                                                                  |
| `gc_time_ratio`                               | Sets the target ratio of time that the JVM allows to spend on garbage collection relative to application runtime. | `int`      | Leave default.<br>0 - Disabled.                                                                                                                                                      |
| `use_dynamic_number_of_gc_threads`            | Allows the JVM to automatically change the number of garbage collector threads depending on load.                 | `boolean`  | Set to `true`. Usually improves frame time stability as GC adapts to load, but on weak CPUs may sometimes add slight delay variability.                                              |
| `use_string_deduplication`                    | Enables string deduplication in G1 GC, combining identical strings into a single copy in memory.                  | `boolean`  | Set to `true`, reduces heap memory consumption and may reduce GC pressure, but adds slight background CPU overhead, usually a beneficial tradeoff for long gaming sessions.          |
  
## GC Threads
  
| Parameter                       | Description                                                                                                              | Type   | Recommendation                                                                                                                                  |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `parallel_gc_threads`           | Sets the number of threads used by the garbage collector for parallel work.                                              | `int`  | -1 - automatic detection.<br>Usually set to the number of physical CPU cores.                                                                   |
| `conc_gc_threads`               | Sets the number of threads for background phases of the garbage collector.                                               | `int`  | Set to ~25% of `parallel_gc_threads`, rounding down. But not less than 1.                                                                       |
| `soft_ref_lru_policy_ms_per_mb` | Sets how long the JVM "tolerates" soft references in memory before cleaning them (milliseconds per 1 MB of free memory). | `int`  | ~50–200 to reduce frequent cache cleaning and reduce microfreezes. But too high values can bloat memory usage and increase GC pause risk.       |
  
## JIT Compilation

| Parameter                         | Description                                                                                                  | Type       | Recommendation                                                                                                                                                                 |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `reserved_code_cache_size_mb`     | Sets the maximum amount of memory allocated by the JVM for compiled JIT code cache in MB.                    | `int`      | For long-lived applications like STALCRAFT: X, it is recommended to set ~256-512 MB.                                                                                           |
| `max_inline_level`                | Limits the depth of nested method inlining during JIT compilation.                                           | `int`      | ~12-15 for CPU-intensive applications. Higher values require more JIT cache.                                                                                                   |
| `freq_inline_size`                | Maximum size of a method that the JVM considers "frequently called" and can aggressively inline.             | `int`      | ~300-500 for hot code paths. Higher values require more JIT cache.                                                                                                             |
| `inline_small_code`               | Sets the size threshold below which a compiled method is considered "small" and inlines more easily.         | `int`      | ~2000–3000 to increase inlining aggressiveness. Value 0 disables the threshold. Higher values require more JIT cache.                                                          |
| `max_node_limit`                  | Limits the maximum number of nodes in the compiler IR graph for one method.                                  | `int`      | ~100 000-240 000. Higher values may cause microfreezes.                                                                                                                        |
| `node_limit_fudge_factor`         | Sets an allowance above `max_node_limit`, allowing the compiler to raise it during optimizations.            | `int`      | ~2000-8000. Change only together with `max_node_limit`. <br>With `max_node_limit`= 240000 can set 8000.<br>0 - Disables increase. High values may cause microfreezes.          |
| `nmethod_sweep_activity`          | Regulates the intensity of code cache cleanup from outdated JIT-compiled methods.                            | `int`      | ~1-4 for stability during long gameplay. Too aggressive cleanup can cause recompilations and FPS drops.                                                                        |
| `dont_compile_huge_methods`       | Prevents JIT compilation of excessively large methods.                                                       | `boolean`  | Set to `false` on very powerful CPUs. In all other cases `true`.                                                                                                               |
| `allocate_prefetch_style`         | Sets the strategy for hardware/software prefetch when allocating memory to objects in the JVM.               | `int`      | ~1-3 depending on CPU power. Reduces memory access delays and can slightly improve frame smoothness.<br>0 - standard strategy.                                                 |
| `always_act_as_server_class`      | Forces the JVM to behave like a server machine and use server JIT optimizations even on client systems.      | `boolean`  | Set to `true` on multi-core (8+) CPUs. Otherwise `false`. May increase game startup time.                                                                                      |
| `use_xmm_for_array_copy`          | Enables SIMD for accelerated array copying in the JVM.                                                       | `boolean`  | Set to `true`, improves frame times, reduces CPU load. Except on ancient processors.                                                                                           |
| `use_fpu_for_spilling`            | Allows the JVM to use FPU/SSE registers for temporary storage of values when CPU registers are insufficient. | `boolean`  | Set to `true`. Offloads main registers, stabilizes frame time in CPU-intensive scenarios.                                                                                      |
  
## Other

| Parameter         | Description                                                                     | Type       | Recommendation                                      |
| ----------------- | ------------------------------------------------------------------------------- | ---------- | --------------------------------------------------- |
| `use_large_pages` | Flag for using Large Pages. Requires policy configuration through `secpol.msc`. | `boolean`  | Set to `true` when system resources are sufficient. |