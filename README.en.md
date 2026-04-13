# Stalcraft JVM Wrapper

[![eng](https://img.shields.io/badge/lang-English-blue)](README.en.md)
[![ru](https://img.shields.io/badge/lang-Russian-blue)](README.md)

> [!WARNING]
> This project is an **unofficial** utility developed by [SilentBless](https://github.com/SilentBless).
> The utility **is not affiliated with EXBO**, but has been verified by [GloomyFolken](https://github.com/GloomyFolken)
> and classified as safe software.

**A utility for modifying JVM startup parameters and optimizing its performance.**

**JVM (Java Virtual Machine)** is the runtime environment through which [STALCRAFT: X](https://stalcraft.net/) operates.

The game code is executed not directly on the system, but inside a Java virtual machine. During execution, it compiles the code into machine code specific to your PC (JIT compilation). Essentially, this is an additional layer between the game and hardware that is responsible for executing the code and adapting it to the system.

This program allows you to change JVM startup parameters to increase game performance, using both preset and custom JSON configuration files.

> [!IMPORTANT]
> The utility tunes JVM parameters for any amount of RAM starting from 8 GB.
> On systems with less RAM the generated `default.json` uses a minimally safe heap,
> but stable gameplay is not guaranteed — prefer upgrading your RAM or sticking with
> the stock EXBO launcher settings.

[![Downloads](https://img.shields.io/github/downloads/EXBO-Community/stalcraft-jvm-optimization/total?label=Downloads&color=green)](../../releases)
[![Latest Release](https://img.shields.io/github/v/release/EXBO-Community/stalcraft-jvm-optimization?label=Latest)](../../releases/latest)

---

## Changes Made

The utility ships as two binaries that must live in the same directory:

- **`cli.exe`** — the interactive menu for installing, removing and managing configurations. The user only launches this when they need to change something.
- **`service.exe`** — the silent interceptor that Windows spawns automatically when the game starts. It has no UI, and you never run it by hand.

`service.exe` intercepts the startup of the game process `stalcraft.exe` (launcher) or `stalcraftw.exe` (Steam) to:

- **Select optimal JVM configuration:** allocated resources volume, Garbage Collector (GC) mode, and JIT compilation mode.
- **Increase game process priority:** the process runs with higher priority compared to other processes.

The utility is installed **once** and automatically runs each time the game is launched.

> [!IMPORTANT]
> Game files are not affected or modified.
> The utility does not interfere with the game process and is not embedded in it.

## System Requirements

- **Operating System:** Windows 10/11
- **Game Version:** Steam/Launcher/EGS/VK Play
- **OS Rights:** administrator privileges in Windows (only required during install/uninstall)
- **CPU:** 4 or more cores
- **RAM:** 8+ GB, 12+ GB recommended (below 12 GB some optimizations such as `PreTouch` stay disabled)

## Using the Utility

### Installation

1. Add the game folder to Windows Defender exclusions or your antivirus software:
    - Example for Steam: `C:\Program Files\Steam\steamapps\common\STALCRAFT`
    - Example for Launcher: `C:\Users\User\AppData\Roaming\EXBO`
    - Example for EGS: `C:\Games\EGS Stalcraft\STALCRAFT`
2. Create a `jvm_wrapper` directory inside the game folder.
3. Download the [latest release](../../releases/latest) and extract `wrapper.zip` into `jvm_wrapper` — you should end up with `cli.exe`, `service.exe` and an `examples/` directory inside.
4. Run `cli.exe`, select `Install` in the menu using the arrow keys and press **Enter**.
5. A UAC prompt will appear — accept it. This is expected: the IFEO hook is written to `HKLM` which requires administrator privileges.

**Now you can launch the game!**

> [!IMPORTANT]
> A few notes on how the utility behaves:
>
> - Hardware G-Sync may cause image artifacts. Disabling it is recommended.
> - The utility only applies to STALCRAFT and does not touch other JVM applications.
> - On systems with 8-16 GB of RAM, it is recommended to keep the Windows page file enabled.

### Uninstallation

1. Run `cli.exe`, select `Uninstall` in the menu using the arrow keys and press **Enter**.
2. Navigate to the game folder.
3. Delete the `jvm_wrapper` folder.
4. Restart the game if it is running.

### Configuration

After installation, the utility will automatically create a `default.json` configuration profile,
which will be located in the `jvm_wrapper/configs/default.json` folder.
The game will launch with this profile by default.
This profile will be adapted to your computer's parameters, but its existence does not preclude custom configuration.

**Configuration is saved in the Windows registry:** `HKCU\\Software\\StalcraftWrapper`.

You can change the launch configuration yourself. To do this:

1. Run `cli.exe`, select `Select Config` in the menu using the arrow keys and press **Enter**.
2. Select the desired configuration file and press **Enter**.
3. Restart the game if it is running.

> [!NOTE]
> By default only the `default.json` configuration is available, but it is *not* the only option.
> See the [Example configurations](#example-configurations) and [Custom configuration](#custom-configuration)
> sections below for instructions.

#### Example configurations

The repository currently ships one example — `examples/8khz.json`, targeted at high-end systems (8+ cores, 32 GB RAM) running 8 kHz mice. It prioritizes minimal STW pauses and predictable frame time at the cost of a small amount of throughput.

To use an example, browse the [`/examples`](./examples/) directory in this repository, download the `.json` you want and drop it into `jvm_wrapper/configs/`.

Then run the utility, pick `Select Config` in the menu. A new profile should appear alongside `default.json` — select it, then restart the game.

#### Custom Configuration

To create your own configuration profile, simply copy the `default.json` file,
rename it to something like `my_setup.json`, then edit it with any available
text editor.

> [!CAUTION]
> Custom configuration is recommended only for those who **100% understand** what they are doing.
> Otherwise, you risk compromising not only JVM stability and, as a consequence, the game, but
> also your entire operating system.

Creating your own configuration should be accompanied by studying the [documentation](./docs/PARAMS.en.md)
on configuration parameters.

> [!TIP]
> If you've customized the configuration in `default.json` and want to revert
> to the recommended settings — select `Regenerate Config` in the menu.
> This action will write the optimal settings for your PC to `default.json`.

---

## Additional Information

### Logging

Both binaries write structured logs into `jvm_wrapper/logs/` next to `cli.exe` and `service.exe`:

- **`logs/wrapper.log`** — wrapper events: startup, hardware detection, config load, game process spawn, exit code. User profile paths are redacted to `<user>`, raw launcher arguments and JVM flags are never written. The file is truncated once it exceeds 2 MB.
- **`logs/jvm.log`** — [JDK 9 unified logging](https://openjdk.org/jeps/158): GC pauses, every STW safepoint, JIT compilation, deoptimization, metaspace and code cache events. 3 rotated files of 10 MB each, up to 30 MB on disk total. Used to diagnose the root cause of microfreezes.

If you run into a problem and want to report it, attach both files from `jvm_wrapper/logs/` to your GitHub issue. They contain no personal information and are safe to publish.

### Large Pages

**Large Pages** is a virtual memory mode where larger pages are used instead of the standard 4 KB.

Enabling Large Pages reduces memory access overhead, making GC and heap access smoother and faster. The CPU does not access RAM directly — it goes through the TLB (Translation Lookaside Buffer); fewer TLB misses mean higher throughput.

> [!CAUTION]
> Large Pages lock memory to the application and prevent the system from reallocating it.
> Incorrect configuration can lead to unstable OS operation. Be aware of your actions!
> Make sure that the allocated memory in your configuration profile does not exceed 40%-50% of total RAM,
> and that you have at least 16+ GB of RAM.

To enable Large Pages, follow these steps:

1. Press `Win` + `R`.
2. Type `secpol.msc` and press `Enter`.
3. Navigate to *Local Policies → User Rights Assignment*.
4. Find the *"Lock pages in memory"* policy.
5. Double-click it and add your user account or the "Administrators" group.
6. Apply the changes and log out / log back in for the policy to take effect.

### Technical Information

Detailed technical information describing the utility's operating principles,
as well as build instructions can be found [here](./docs/OVERVIEW.en.md).
