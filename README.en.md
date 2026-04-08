# stalcraft-wrapper

[![Downloads](https://img.shields.io/github/downloads/EXBO-Community/stalcraft-jvm-optimization/total?label=Downloads&color=green)](../../releases)
[![Latest Release](https://img.shields.io/github/v/release/EXBO-Community/stalcraft-jvm-optimization?label=Latest)](../../releases/latest)

> **Disclaimer:** This is an **unofficial** project created by [SilentBless](https://github.com/SilentBless). We thank them for their work on the project. The project is **not supported by or affiliated with EXBO**, but it has been officially reviewed for safety on your PC.

JVM wrapper for STALCRAFT. Automatically optimizes Java settings for your hardware for better performance.

> **Note:** On systems with 8 GB of RAM or less, the wrapper does not inject flags — the default launcher settings are sufficient, and aggressive tuning on low memory can hurt performance.

## What it does

- Tunes Java settings (memory, garbage collector, threads) for your PC
- Boosts game process priority
- Install once — works automatically on every launch
- No game files modified
- JSON config support with fine-grained tuning

## Installation

1. Download `wrapper.exe` from [Releases](../../releases)
2. Place it anywhere
3. Run as administrator

A menu will appear:

```
  > Install
    Uninstall
    Status
    Select Config
    Regenerate Config
    Exit
```

Select **Install** with arrow keys, press Enter. Done.

Both game versions are supported:
- `stalcraft.exe` (main launcher)
- `stalcraftw.exe` (Steam)

## Uninstall

Run `wrapper.exe` as admin and select **Uninstall**.

## Configuration

On first game launch, the wrapper auto-generates `configs/default.json` with optimal settings for your hardware.

### Preset profiles

The `configs/` folder includes ready-made profiles:

| Profile | Description |
|---------|-------------|
| `weak.json` | 4 cores, 8-12 GB RAM — minimal CPU overhead |
| `medium.json` | 6 cores, 16 GB RAM — balanced performance |
| `max.json` | 8+ cores, 32+ GB RAM — maximum optimization |

### Selecting a config

Via menu: **Select Config** &rarr; arrow keys to choose &rarr; Enter.

The active config is stored in the registry (`HKCU\Software\StalcraftWrapper`) and can be changed at any time.

### Custom config

1. Copy any `.json` from `configs/`
2. Rename it (e.g. `my_setup.json`)
3. Edit the parameters
4. Select it via **Select Config** in the menu

### Regenerate Config

Recreates `default.json` based on current hardware (useful after an upgrade).

## Large Pages (optional)

For additional performance, enable large pages:

1. Open `secpol.msc`
2. Local Policies &rarr; User Rights Assignment &rarr; Lock pages in memory
3. Add your user, reboot

The wrapper will detect and enable this automatically.

## Requirements

- Windows 10/11
- Administrator privileges (for install/uninstall)

---

## Technical Details

### How it works

The wrapper uses [IFEO](https://learn.microsoft.com/en-us/previous-versions/windows/desktop/xperf/image-file-execution-options) to intercept game launch. When `stalcraft.exe` / `stalcraftw.exe` starts, Windows redirects the call through the wrapper, which:

1. Loads the active config from `configs/` (or auto-generates one)
2. Strips conflicting flags from the original launcher arguments
3. Launches the process directly via `ntdll!NtCreateUserProcess`, bypassing repeated IFEO interception
4. Sets elevated memory and I/O priority via `NtSetInformationProcess`
5. Exits after the game's first visible window appears

### CLI

```
wrapper.exe --install     # register IFEO hook
wrapper.exe --status      # check status
wrapper.exe --uninstall   # remove IFEO hook
```

### Build

```
cd wrapper
go build -o wrapper.exe -ldflags="-s -w" .
```

---

## Config Parameters

### Memory

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `heap_size_gb` | Heap size (Xmx/Xms) in GB | 4-8 depending on free RAM |
| `pre_touch` | Pre-touch all heap memory at startup (`AlwaysPreTouch`) | `true` on 8+ cores, otherwise `false` — faster runtime, slower startup |
| `metaspace_mb` | Class metadata size in MB | 128 (heap &le;4g), 256 (heap &le;8g), 512 (heap >8g) |

### G1GC — Core

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `max_gc_pause_millis` | Target max GC pause in ms. G1 will try not to exceed this | 50 — balance between pause frequency and length |
| `g1_heap_region_size_mb` | Size of one G1 region in MB. Objects >= half a region are "humongous" | 8 (heap &le;4g), 16 (heap &le;8g), 32 (heap >8g) |
| `g1_new_size_percent` | Min % of heap for young generation | 23 — enough for allocations without overflow |
| `g1_max_new_size_percent` | Max % of heap for young generation | 40-50 — higher = less frequent minor GC, but longer pauses |
| `g1_reserve_percent` | % of heap reserved to protect against to-space exhaustion | 20 — buffer for peak allocations |
| `g1_heap_waste_percent` | Tolerable % of garbage in heap before mixed GC | 5 — lower = cleaner heap, but more frequent mixed GC |
| `g1_mixed_gc_count_target` | Number of cycles to spread mixed GC over | 3-4 — fewer = faster cleanup, but longer each pause |
| `initiating_heap_occupancy_percent` | Heap fill % to start concurrent marking | 15 (strong CPU) / 30 (weak) — lower = earlier start, less full GC risk |
| `g1_mixed_gc_live_threshold_percent` | Only include regions in mixed GC if < X% live objects | 90 — only collect heavily polluted regions |
| `g1_rset_updating_pause_time_percent` | % of pause spent updating remembered sets | 0 (strong CPU — all concurrent) / 5-10 (weak) |
| `survivor_ratio` | Eden to Survivor ratio in young generation | 32 — large Eden, objects die fast or go straight to old gen |
| `max_tenuring_threshold` | GC cycles before promoting from young to old gen | 1 — for games, objects either die immediately or live forever |

### G1GC — Advanced (STW minimization)

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `g1_satb_buffer_enqueuing_threshold_percent` | SATB buffer fill % to start processing | 30 — earlier processing = less STW work. 0 = disabled |
| `g1_conc_rs_hot_card_limit` | Hot card limit for concurrent refinement | 16 — more concurrent work, less STW. 0 = disabled |
| `g1_conc_refinement_service_interval_millis` | Concurrent refinement interval in ms | 150 — smoother background load distribution. 0 = disabled |
| `gc_time_ratio` | App-to-GC time ratio (N means 1/(1+N) time for GC) | 99 (strong — 99% to app) / 19 (weak — default). 0 = disabled |
| `use_dynamic_number_of_gc_threads` | Dynamically adjust GC thread count | `true` on 8+ cores |
| `use_string_deduplication` | Deduplicate identical strings in heap | `true` on 8+ cores — saves memory, adds GC load |

### GC Threads

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `parallel_gc_threads` | Threads for STW pauses (app is frozen — can use more) | cores / 2, min 2 |
| `conc_gc_threads` | Threads for background GC (competes with game — keep low) | cores / 4, min 1 |
| `soft_ref_lru_policy_ms_per_mb` | Soft reference lifetime (ms per MB of free heap) | 10-25 — lower = more aggressive cleanup, higher = longer caching |

### JIT Compilation

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `reserved_code_cache_size_mb` | Total code cache size in MB | 256 (weak) / 400 (strong) — stores JIT-compiled code |
| `max_inline_level` | Max inlining depth for method calls | 15 — deeper = faster, but more code cache usage |
| `freq_inline_size` | Max "hot" method size for inlining (bytecode) | 500 — higher = more aggressive inlining |
| `inline_small_code` | Inline methods with native code up to X bytes | 4000 (strong) / 0 = disabled (weak) |
| `max_node_limit` | Max nodes in compilation graph per method | 240000 (strong) / 0 = default (weak). Paired with `node_limit_fudge_factor` |
| `node_limit_fudge_factor` | Allowance above `max_node_limit` (must be 2-40% of it) | 8000 (with max_node_limit=240000). 0 = disabled |
| `nmethod_sweep_activity` | Aggressiveness of stale JIT code cleanup (1-10) | 1 (strong — minimal cleanup) / 0 = default |
| `dont_compile_huge_methods` | Skip compilation of huge methods | `false` on strong CPUs (compile everything), `true` on weak |
| `allocate_prefetch_style` | Hardware prefetch style on allocation (0-3) | 3 (strong — all fields), 0 = disabled |
| `always_act_as_server_class` | Enable server-class JVM optimizations | `true` on 8+ cores |
| `use_xmm_for_array_copy` | Use XMM registers for array copying | `true` on strong CPUs — faster `System.arraycopy` |
| `use_fpu_for_spilling` | Use FPU registers for spilling intermediate values | `true` on strong CPUs — offloads general registers |

### Other

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `use_large_pages` | Use Large Pages (requires OS setup) | `true` if `SeLockMemoryPrivilege` is configured |

## License

MIT
