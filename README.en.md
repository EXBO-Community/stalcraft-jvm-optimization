# stalcraft-wrapper

JVM wrapper for STALCRAFT. Automatically optimizes Java settings for your hardware for better performance.

## What it does

- Tunes Java settings (memory, garbage collector, threads) for your PC
- Boosts game process priority
- Install once — works automatically on every launch
- No game files modified

## Installation

1. Download `wrapper.exe` from [Releases](../../releases)
2. Place it anywhere
3. Run as administrator

A menu will appear:

```
  > Install
    Uninstall
    Status
    Exit
```

Select **Install** with arrow keys, press Enter. Done.

Both game versions are supported:
- `stalcraft.exe` (main launcher)
- `stalcraftw.exe` (Steam)

## Uninstall

Run `wrapper.exe` as admin and select **Uninstall**.

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

1. Detects hardware: RAM, CPU, large pages (`GlobalMemoryStatusEx`, `GetLargePageMinimum`)
2. Generates JVM flags for the current configuration
3. Strips conflicting flags from the original launcher arguments
4. Launches the process directly via `ntdll!NtCreateUserProcess`, bypassing repeated IFEO interception
5. Sets elevated memory and I/O priority via `NtSetInformationProcess`
6. Exits after the game's first visible window appears

### IFEO bypass

The process is created via `NtCreateUserProcess` (ntdll) directly, bypassing `CreateProcessInternalW` (kernel32) where the IFEO check occurs. The `IFEOSkipDebugger` bit is also set in `PS_CREATE_INFO` for defense in depth.

### Dynamic flag tuning

| Parameter | Formula |
|-----------|---------|
| Heap | 50% free RAM, floor 25% total, cap min(16g, 75% total) |
| ParallelGCThreads | cores - 2, min 2 |
| ConcGCThreads | parallel / 4, min 1 |
| G1HeapRegionSize | 4m / 8m / 16m / 32m based on heap |
| Metaspace | 128m / 256m / 512m based on heap |
| CodeCache | heap/16, clamped 128-512m |
| SurvivorRatio | 32 (&le;4 cores) / 8 (>4 cores) |
| Large Pages | automatic when privilege available |

On systems with &le;8GB RAM, flags are not injected.

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

## License

MIT
