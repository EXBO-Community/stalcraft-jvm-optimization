# Technical Information

## Architecture

The project ships two binaries that must live in the same directory:

- `cli.exe` — user-facing entry point. Interactive menu, install/uninstall of the IFEO hook, status checks, config management.
- `service.exe` — silent interceptor. Registered as the IFEO `Debugger` for `stalzone.exe` / `stalzonew.exe` and spawned by Windows automatically when the game launches. Has no UI.

On install, `cli.exe` writes the path to `service.exe` (not itself) into the registry. The split keeps the Windows → game path running through a minimal UI-free binary while all management stays in a separate `cli.exe`.

## Operating Mechanism

The wrapper uses the IFEO (Image File Execution Options) mechanism to intercept game startup.
When `stalzone.exe` / `stalzonew.exe` is launched, Windows starts `service.exe` instead, passing it the original launcher arguments. `service.exe` then:

1. Loads the active configuration file from the `configs/` directory next to the executable.
2. Validates independent game-region tunnel overrides from `overrides.json` without making network requests.
3. Strips conflicting flags from the original launcher arguments and injects the hardware-tuned JVM flags plus one saved `-Droxy_address_override.<region>` property for every configured region.
4. Creates the process directly through `ntdll!NtCreateUserProcess` with the `PS_ATTRIBUTE_IFEO_SKIP_DEBUGGER` attribute to avoid re-interception through IFEO.
5. Exits as soon as the game process shows its first visible window.

## Tunnel Override

Roxy catalogs are requested over HTTPS only when their region is opened in the TUI. The primary address and its mirrors are attempted sequentially as fallbacks, with `login=EXBO-Community` added to every request.

For a measurement, `cli.exe` sends a random 16-byte UUID to `tunnel_port + 1`. The response must arrive within one second and use `UUID[16] | tunnelToBackendRtt:i32 (big-endian) | limitReached:u8`. Client RTT is measured locally. Every endpoint in an opened group, or every endpoint outside excluded groups during a search, is measured concurrently and appears in the UI as soon as it replies. There are no automatic repeat rounds; another probe starts only after reopening the view or explicitly choosing `Measure again`.

Settings live in `overrides.json`, separately from versioned JVM profiles. Overrides for RU, EU, NA, SEA, and NEA are stored independently and can be active at the same time; `Game default` clears only the current region. The `cache/tunnel_stats.json` cache keeps up to 20 measurements per endpoint together with the last connection-limit state and its timestamp; entries older than 24 hours are removed. Nodes that reported a limit last time are measured first and remain unavailable for a new selection until they reply with `limitReached=false`.

## Logging

Both binaries write structured logs into `logs/wrapper.log` next to the executable: startup, hardware detection, config load, game process spawn, exit code. User profile paths are redacted, raw launcher arguments and JVM flags are never logged. The file is truncated once it exceeds 2 MB.

There is no separate JVM/GC log file — STALZONE bundles a custom OpenJDK 9 build whose CLI parsers for `-Xlog` and `-Xloggc` have been stripped, so unified logging cannot be directed to a file. `wrapper.log` is enough for the vast majority of support cases.

## CLI Interaction

Installing the IFEO interception

```bash
cli.exe install       # install IFEO interception
```

Checking interception status

```bash
cli.exe status        # check interception status
```

Removing the IFEO interception

```bash
cli.exe uninstall     # remove IFEO interception
```

Config management

```bash
cli.exe config list
cli.exe config releases
cli.exe config regenerate v1.1.2
cli.exe config select v1.1.2/default
```

Running `cli.exe` without arguments opens the interactive menu that exposes the same actions plus config management.

## Building the Project

`cli.exe` and `service.exe` can be downloaded from the releases page or built locally.
From the repository root:

```bash
mkdir -p build
version="$(git describe --tags --always --dirty)"
commit="$(git rev-parse --short HEAD)"
ldflags="-s -w -X github.com/EXBO-Community/stalcraft-jvm-optimization/internal/buildinfo.Version=${version} -X github.com/EXBO-Community/stalcraft-jvm-optimization/internal/buildinfo.Commit=${commit}"
go build -trimpath -ldflags="${ldflags}" -o build/cli.exe     ./cmd/cli
go build -trimpath -ldflags="${ldflags}" -o build/service.exe ./cmd/service
```

Drop both binaries into the same directory before running — the installer is only in `cli.exe`, but it looks for `service.exe` next to itself.
