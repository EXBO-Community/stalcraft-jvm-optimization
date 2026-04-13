# Technical Information

## Architecture

The project ships two binaries that must live in the same directory:

- `cli.exe` — user-facing entry point. Interactive menu, install/uninstall of the IFEO hook, status checks, config management.
- `service.exe` — silent interceptor. Registered as the IFEO `Debugger` for `stalcraft.exe` / `stalcraftw.exe` and spawned by Windows automatically when the game launches. Has no UI.

On install, `cli.exe` writes the path to `service.exe` (not itself) into the registry. The split keeps the Windows → game path running through a minimal UI-free binary while all management stays in a separate `cli.exe`.

## Operating Mechanism

The wrapper uses the IFEO (Image File Execution Options) mechanism to intercept game startup.
When `stalcraft.exe` or `stalcraftw.exe` is launched, Windows starts `service.exe` instead, passing it the original launcher arguments. `service.exe` then:

1. Loads the active configuration file from the `configs/` directory next to the executable.
2. Strips conflicting flags from the original launcher arguments and injects hardware-tuned JVM flags.
3. Creates the process directly through `ntdll!NtCreateUserProcess` with the `PS_ATTRIBUTE_IFEO_SKIP_DEBUGGER` attribute to avoid re-interception through IFEO.
4. Raises memory and I/O priorities of the new process via `NtSetInformationProcess`.
5. Exits as soon as the game process shows its first visible window.

## Logging

Both binaries write structured logs into the `logs/` directory next to the executable:

- `logs/wrapper.log` — wrapper events: startup, hardware detection, config load, process start, exit code. User profile paths are redacted, raw launcher arguments and JVM flags are never logged. The file is truncated once it exceeds 2 MB.
- `logs/jvm.log` — JDK 9+ unified logging: GC pauses, all STW safepoints, JIT compilation, deoptimization, metaspace and code cache events. Rotated into 3 files of 10 MB each. Use it to diagnose microfreezes.

## CLI Interaction

Installing the IFEO interception

```bash
cli.exe --install     # install IFEO interception
```

Checking interception status

```bash
cli.exe --status      # check interception status
```

Removing the IFEO interception

```bash
cli.exe --uninstall   # remove IFEO interception
```

Running `cli.exe` without arguments opens the interactive menu that exposes the same actions plus config management.

## Building the Project

`cli.exe` and `service.exe` can be downloaded from the releases page or built locally.
From the repository root:

```bash
mkdir -p build
go build -trimpath -ldflags="-s -w" -o build/cli.exe     ./cmd/cli
go build -trimpath -ldflags="-s -w" -o build/service.exe ./cmd/service
```

Drop both binaries into the same directory before running — the installer is only in `cli.exe`, but it looks for `service.exe` next to itself.
