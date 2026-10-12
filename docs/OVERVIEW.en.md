# Technical Information

## Architecture

The project ships two binaries that must live in the same directory:

- `cli.exe` — user-facing entry point. Interactive menu, install/uninstall of the IFEO hook, status checks, config management.
- `service.exe` — silent interceptor. Registered as the IFEO `Debugger` for `stalzone.exe` / `stalzonew.exe` and spawned by Windows automatically when the game launches. Has no UI.

On install, `cli.exe` writes the path to `service.exe` (not itself) into the registry. The split keeps the Windows → game path running through a minimal UI-free binary while all management stays in a separate `cli.exe`.

## Operating Mechanism

The wrapper uses the IFEO (Image File Execution Options) mechanism to intercept game startup.
When `stalzone.exe` / `stalzonew.exe` is launched, Windows starts `service.exe` instead, passing it the original launcher arguments. `service.exe` then:

1. Loads the active configuration file from `configs/` next to the executable; with `ActiveConfig=<none>`, no profile is loaded or generated.
2. Validates independent game-region tunnel overrides from `overrides.json` without making network requests.
3. With optimization enabled, replaces conflicting JVM flags with the profile. Without optimization, preserves launcher arguments and replaces only selected `-Droxy_address_override.<region>` properties.
4. Creates the process directly through `ntdll!NtCreateUserProcess` with the `PS_ATTRIBUTE_IFEO_SKIP_DEBUGGER` attribute to avoid re-interception through IFEO.
5. Exits as soon as the game process shows its first visible window.

## Tunnel Override

Roxy catalogs are requested over HTTPS only when their region is opened in the TUI. The primary address and its mirrors are attempted sequentially as fallbacks, with `login=EXBO-Community` added to every request.

For a measurement, `cli.exe` sends a random 16-byte UUID to `tunnel_port + 1`. The response must use `UUID[16] | tunnelToBackendRtt:i32 (big-endian) | limitReached:u8`. Client RTT is measured locally. At most eight network probes run concurrently, and each has a one-second deadline after admission to that limit; waiting in the queue does not consume the deadline. Replies must match the request UUID and the current UI round; restarting cancels the previous round. Opening a group probes its nodes and shows replies as they arrive. There are no background repeat rounds; fresh measurements start only after reopening the view or explicitly choosing `Measure again`.

Settings live in `overrides.json`, separately from versioned JVM profiles. Overrides for RU, EU, NA, SEA, and NEA are stored independently and can be active at the same time; `Game default` clears only the current region. The `cache/tunnel_stats.json` cache keeps up to 20 measurements per endpoint together with the last connection-limit state and its timestamp; entries older than 24 hours are removed. Nodes that reported a limit last time are measured first and remain unavailable for a new selection until they reply with `limitReached=false`.

Each search collects a fresh series of 10 probes for every endpoint outside excluded groups, with 100 ms between probes for the same endpoint. Refresh replaces these series rather than adding previous search samples. A larger endpoint pool or nodes that do not reply can make a search take longer because probes share the concurrency limit and unsuccessful probes can use the full deadline. History is supplemental only and does not enter the search score.

Search keeps every ranked endpoint. Five candidate rows are shown at a time, with arrow-key scrolling to the rest; shorter terminals use a smaller window. Results are explicitly preliminary while any endpoint series is incomplete or a candidate has fewer than five successful replies. `Use best` is disabled until all endpoint series finish and does not automatically select a candidate with fewer than five successful replies. Such a candidate may still be chosen manually, with the failure and sample counts visible. Every selection requires a fresh successful reply without a connection limit.

For each successful probe, the selected mode defines a latency value: client RTT + server RTT for total RTT; client RTT alone for client mode; server RTT alone for server mode; or client RTT × the backend-provided client weight + server RTT for backend-weighted mode. The median and P90 are computed from those per-probe values, not from separate component averages. With `f = failed probes / attempted probes`, the score is:

`median + 0.5 × (P90 − median) + ProbeTimeout_ms × f`

Stability mode instead prioritizes the upper end of the total-RTT distribution:

`P90(client RTT + server RTT) + ProbeTimeout_ms × f`

`ProbeTimeout_ms` is 1000. Lower scores rank first. Failed probes contribute to the penalty, not to the successful latency distribution. Result details show the current series' total-RTT median and P90, failed-probe percentage, successful/attempted counts, and the last successful raw client, server, and total RTTs. The score is a comparison value, not measured game ping; failed-probe percentage is not game packet loss. A small number of replies does not establish statistical confidence.

The series maximum is also shown: a single huge reply stays visible even when it does not affect the median or P90.

**RTT provenance:** server RTT is the signed millisecond value reported by the tunnel, not a second measurement performed by this PC. A large non-negative server value is preserved without arbitrary clipping; the robust score does not rewrite raw RTTs. Historical arithmetic averages can remain high after a large reply, but previous rounds do not influence a fresh search score. Accepted replies are logged as `tunnel probe reply` with region, group, node name, round, RTT values, and limit state; UUIDs and raw packets are not logged. These diagnostics identify the source of a displayed value, but cannot verify whether the tunnel's backend RTT report is accurate.

A very short local RTT can read as zero at the system clock's resolution. This does not mean zero physical latency; the measured value is not replaced with an artificial minimum.

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
cli.exe config disable
```

Running `cli.exe` without arguments opens the interactive menu that exposes the same actions plus config management.

## Building the Project

`cli.exe` and `service.exe` can be downloaded from the releases page or built locally.

Git tags are required to build the configurations correctly. Once built, the compiled binaries do not access Git.

There is a chance that the tags may be missing. To check, run: `git tag`. To download them to your computer, follow the commands below.

If you cloned [the main repository](https://github.com/EXBO-Community/stalcraft-jvm-optimization):
```bash
git fetch origin --tags
```
If you are working in your fork, first add it as a remote source:
```bash
git remote add upstream https://github.com/EXBO-Community/stalcraft-jvm-optimization
git fetch upstream --tags
```
Now you can proceed to the build itself.
From the repository root:

```bash
mkdir -p build
go generate ./...
go build -trimpath -ldflags="-s -w" -o build/cli.exe     ./cmd/cli
go build -trimpath -ldflags="-s -w" -o build/service.exe ./cmd/service
```

Drop both binaries into the same directory before running — the installer is only in `cli.exe`, but it looks for `service.exe` next to itself.

Run `bash scripts/check.sh` locally and in Windows CI for gofmt, go vet, builds, and race-enabled tests. On macOS/Linux it runs portable tests and a Windows cross-build; full UI and registry behavior require Windows.

Git preserves LF endings for `.go` and `.sh` through `.gitattributes`, including Windows checkouts with `core.autocrlf=true`. This keeps `gofmt` checks and Bash scripts consistent across platforms.
