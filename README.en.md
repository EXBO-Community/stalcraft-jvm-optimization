# Stalcraft JVM Wrapper

[![eng](https://img.shields.io/badge/lang-English-blue)](README.en.md)
[![ru](https://img.shields.io/badge/lang-Russian-blue)](README.md)

> [!WARNING]
> This project is an **unofficial** utility developed by [SilentBless](https://github.com/SilentBless).
> The utility **is not affiliated with EXBO**, but has been verified by [GloomyFolken](https://github.com/GloomyFolken)
> and classified as safe software.

> [!CAUTION]
> If you run into problems after installing this utility, **open the [troubleshooting document](./docs/TROUBLESHOOTING.en.md)** and find your situation there. Every common issue and its fix is documented step by step.
>
> Please **do not bother EXBO moderators, the game's technical support and speakers** with issues related to this utility. They are regular people just like you, and they have no idea what is happening on your computer. Everything you need is in the document linked above — read it before reaching out to anyone.

**A utility for modifying JVM startup parameters and optimizing its performance.**

**JVM (Java Virtual Machine)** is the runtime environment through which [STALZONE](https://stalcraft.net/) operates.

The game code is executed not directly on the system, but inside a Java virtual machine. During execution, it compiles the code into machine code specific to your PC (JIT compilation). Essentially, this is an additional layer between the game and hardware that is responsible for executing the code and adapting it to the system.

This program allows you to change JVM startup parameters to increase game performance, using both preset and custom JSON configuration files.

> [!IMPORTANT]
> The utility tunes JVM parameters for any amount of RAM starting from 8 GB.
> On systems with less RAM the generated profile uses a minimally safe heap,
> but stable gameplay is not guaranteed — prefer upgrading your RAM or sticking with
> the stock EXBO launcher settings.

[![Downloads](https://img.shields.io/github/downloads/EXBO-Community/stalcraft-jvm-optimization/total?label=Downloads&color=green)](../../releases)
[![Latest Release](https://img.shields.io/github/v/release/EXBO-Community/stalcraft-jvm-optimization?label=Latest)](../../releases/latest)

---

## Changes Made

The utility ships as two binaries that must live in the same directory:

- **`cli.exe`** — the interactive menu for installing, removing and managing configurations. The user only launches this when they need to change something.
- **`service.exe`** — the silent interceptor that Windows spawns automatically when the game starts. It has no UI, and you never run it by hand.

`service.exe` intercepts the startup of the game process `stalzone.exe` (launcher/EGS/VK Play) or `stalzonew.exe` (Steam) to:

- **Select optimal JVM configuration:** allocated resources volume, Garbage Collector (GC) mode, and JIT compilation mode.
- **Increase game process priority:** the process runs with higher priority compared to other processes.

The utility is installed **once** and automatically runs each time the game is launched.

> [!IMPORTANT]
> Game files are not affected or modified.
> The utility does not interfere with the game process and is not embedded in it.

## System Requirements

- **Operating System:** Windows 10/11
- **Game Version:** Steam/Launcher/EGS/VK Play
- **OS Rights:** administrator privileges in Windows (only required during install)
- **CPU:** 4 or more cores
- **RAM:** 8+ GB, 12+ GB recommended (below 12 GB some optimizations such as `PreTouch` stay disabled)

## Using the Utility

### Installation

> [!TIP]
> The most common mistake during install is dropping `jvm_wrapper` somewhere deep inside `runtime/stalcraft/...`. The folder must sit **at the root of the EXBO directory**, next to `ExboLink.exe` and the `runtime/` directory. Here's what it should look like:
>
> ![Example of where the jvm_wrapper folder belongs at the root of the EXBO launcher directory](./docs/assets/install-folder-location.jpg)

1. Add the game folder to Windows Defender exclusions or your antivirus software:
    - Example for Steam: `C:\Program Files\Steam\steamapps\common\STALCRAFT`
    - Example for Launcher: `C:\Users\User\AppData\Roaming\EXBO`
    - Example for EGS: `C:\Games\EGS Stalcraft\STALCRAFT`
2. Create the `jvm_wrapper` directory at the root of the launcher folder (see the tip above).
3. Download the [latest release](../../releases/latest) and extract `wrapper.zip` into `jvm_wrapper` — you should end up with `cli.exe`, `service.exe` and the `examples/`, `langs/` directories inside.
4. Run `cli.exe`, select `Install` in the menu using the arrow keys and press **Enter**.
5. A UAC prompt will appear — accept it. This is expected: the IFEO hook is written to `HKLM` which requires administrator privileges.

**Now you can launch the game!**

> [!IMPORTANT]
> A few notes on how the utility behaves:
>
> - Hardware G-Sync may cause image artifacts. Disabling it is recommended.
> - The utility only applies to STALZONE and does not touch other JVM applications.
> - On systems with 8-16 GB of RAM, it is recommended to keep the Windows page file enabled.

### Uninstallation

1. Run `cli.exe`, select `Uninstall` in the menu using the arrow keys and press **Enter**.
2. Navigate to the game folder.
3. Delete the `jvm_wrapper` folder.
4. Restart the game if it is running.

### Configuration

After installation, the utility will automatically create a `v1.1.2/default.json` configuration profile,
which will be located at `jvm_wrapper/configs/v1.1.2/default.json`.
The game will launch with this profile by default.
This profile will be adapted to your computer's parameters, but its existence does not preclude custom configuration.

**Configuration is saved in the Windows registry:** `HKCU\\Software\\StalcraftWrapper`.

You can change the launch configuration yourself. To do this:

1. Run `cli.exe`, select `Select Config` in the menu using the arrow keys and press **Enter**.
2. Select the desired configuration file and press **Enter**. You can enter version folders and use `< Back` to move one level up.
3. Restart the game if it is running.

> [!NOTE]
> By default the active configuration is `v1.1.2/default.json`, but it is *not* the only option.
> See the [Example configurations](#example-configurations) and [Custom configuration](#custom-configuration)
> sections below for instructions.

The same operations are available as CLI commands:

- `cli.exe status`
- `cli.exe install`
- `cli.exe uninstall`
- `cli.exe config list`
- `cli.exe config releases`
- `cli.exe config regenerate v1.1.2`
- `cli.exe config select v1.1.2/default`

#### Tunnel Override (Experimental)

The `Tunnel override` menu lets you independently select a specific Roxy node for RU, EU, NA, SEA, and NEA at the same time. `Game default` lives inside each region and clears only that region's override. A regional list is requested only after that region is opened. Opening a group measures all of its nodes concurrently and shows the PC-to-tunnel RTT, tunnel-to-backend RTT, and their sum separately. There are no background repeat rounds; another measurement starts only after reopening the view or selecting `Measure again`.

`Search best server` measures every node outside the persistent exclusion list and shows the best five. Results can be ranked by total ping, client RTT, server RTT, stability, or the weight reported by the game backend. A node that reports a connection limit remains visible, but cannot be selected until a fresh successful measurement clears the limit.

Selections and search settings live in `jvm_wrapper/overrides.json`; up to 20 recent measurements per node live in `jvm_wrapper/cache/tunnel_stats.json` and expire after 24 hours. `Regenerate Config` does not touch either file. At game startup, `service.exe` only validates the stored addresses and adds one JVM property per configured region; it performs no network measurements on the launch path.

#### Example configurations

The repository ships one example: `examples/aikar.json`.
It is inspired by popular Minecraft/Paper/Aikar G1GC tuning ideas, adapted for STALZONE and OpenJDK 9.
It is not an exact copy of server-side Aikar flags and not a universal recommendation.

`aikar.json` uses [PaperMC Aikar flags](https://docs.papermc.io/paper/aikars-flags/) and Prism Launcher's warning that [more allocated RAM does not always mean better performance](https://prismlauncher.org/wiki/help-pages/java-settings/) as reference points.

To use the example, browse the [`/examples`](./examples/) directory in this repository, download `aikar.json` and drop it into `jvm_wrapper/configs/`.

Then run the utility and pick `Select Config` in the menu. A new profile should appear in the list — select it, then restart the game.

#### Custom Configuration

To create your own configuration profile, simply copy the `configs/v1.1.2/default.json` file,
rename it to something like `my_setup.json`, then edit it with any available
text editor.

> [!CAUTION]
> Custom configuration is recommended only for those who **100% understand** what they are doing.
> Otherwise, you risk compromising not only JVM stability and, as a consequence, the game, but
> also your entire operating system.

Creating your own configuration should be accompanied by studying the [documentation](./docs/PARAMS.en.md)
on configuration parameters.

> [!TIP]
> If you've customized a configuration and want to revert
> to the recommended settings — select `Regenerate Config` in the menu and choose the desired version.
> This action will rewrite `configs/<version>/default.json` and make it active.

---

## Additional Information

### Logging

The utility writes a single structured log file at `jvm_wrapper/logs/wrapper.log` next to `cli.exe` and `service.exe`. It records startup, hardware detection, config load, game process spawn and exit code. User profile paths are redacted to `<user>`, raw launcher arguments and JVM flags are **never written**. The file is truncated once it exceeds 2 MB.

If you run into a problem and want to report it, attach this file to your GitHub issue. It contains no personal information and is safe to publish.

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
