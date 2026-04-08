# Stalcraft JVM Wrapper

[![eng](https://img.shields.io/badge/lang-English-blue)](README.en.md)
[![ru](https://img.shields.io/badge/lang-Russian-blue)](README.md)

**A utility for modifying JVM startup parameters and optimizing its performance.**

**JVM (Java Virtual Machine)** is the runtime environment through which [STALCRAFT: X](https://stalcraft.ru/) operates.

The game code is executed not directly on the system, but inside a Java virtual machine. During execution, it compiles the code into machine code specific to your PC (JIT compilation). Essentially, this is an additional layer between the game and hardware that is responsible for executing the code and adapting it to the system.

This program allows you to change JVM startup parameters to increase game performance, using both preset and custom JSON configuration files.

> [!WARNING]
> This project is an **unofficial** utility developed by [SilentBless](https://github.com/SilentBless).
> The utility is not affiliated with EXBO, but has been verified by [GloomyFolken](https://github.com/GloomyFolken)
> and classified as safe software.

> [!IMPORTANT]
> The utility does not modify JVM startup parameters for systems with 8 GB or less of RAM.
> Aggressive optimization with limited memory can harm your PC.
> Instead, use the standard settings from the EXBO launcher.

[![Downloads](https://img.shields.io/github/downloads/EXBO-Community/stalcraft-jvm-optimization/total?label=Downloads&color=green)](../../releases)
[![Latest Release](https://img.shields.io/github/v/release/EXBO-Community/stalcraft-jvm-optimization?label=Latest)](../../releases/latest)

---

## Changes Made

The utility (wrapper) intercepts the startup of the game process `stalcraft.exe` (launcher) or `stalcraftw.exe` (Steam) to:

- **Select optimal JVM configuration:** allocated resources volume, Garbage Collector (GC) mode, and JIT compilation mode.
- **Increase game process priority:** the process runs with higher priority compared to other processes.

The wrapper is installed **once** and automatically runs each time the game is launched.

> [!IMPORTANT]
> Game files are not affected or modified.
> The utility does not interfere with the game process and is not embedded in it.

## System Requirements

- **Operating System:** Windows 10/11
- **Game Version:** Steam/Launcher/EGS
- **OS Rights:** administrator privileges in Windows
- **CPU:** 4 or more cores
- **RAM:** 12+ GB

## Using the Utility

### Installation

1. Add the game folder to Windows Defender exclusions or your antivirus software:
   - Example for Steam: `C:\Program Files\Steam\steamapps\common\STALCRAFT`
   - Example for Launcher: `C:\Users\User\AppData\Roaming\EXBO`
2. Create a `jvm_wrapper` directory in the game folder.
3. Download the [latest version](../../releases/latest) of `wrapper.exe` to the `jvm_wrapper` folder.
4. Run the utility as administrator.
5. In the menu that appears, select `Install` using the arrow keys and press **Enter**.

**Now you can launch the game!**

> [!NOTE]
> The utility only applies to this specific game application using JVM.

### Uninstallation

1. Run the utility as administrator.
2. In the menu that appears, select `Uninstall` using the arrow keys and press **Enter**.
3. Navigate to the game folder.
4. Delete the `jvm_wrapper` folder.
5. Restart the game if it is running.

### Configuration

After installation, the utility will automatically create a `default.json` configuration profile,
which will be located in the `jvm_wrapper/configs/default.json` folder.
The game will launch with this profile by default.
This profile will be adapted to your computer's parameters, but its existence does not preclude custom configuration.

**Configuration is saved in the Windows registry:** `HKCU\\Software\\StalcraftWrapper`.

You can change the launch configuration yourself. To do this:

1. Run the utility as administrator.
2. In the menu that appears, select `Select Config` using the arrow keys and press **Enter**.
3. Select the desired configuration file and press **Enter**.
4. Restart the game if it is running.

> [!NOTE]
> By default, only the `default.json` configuration is available, but it is *not* the only option.
> The sections [Configuration Presets](#configuration-presets) and [Custom Configuration](#custom-configuration)
> provide the necessary instructions.

#### Configuration Presets

The utility repository includes ready-made configuration presets for PCs with different specifications.
Below is a descriptive table for each preset:

| Profile       | Requirements                 | Description                                                                                                                                                                         |
| ------------- | ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `weak.json`   | CPU: 4+ cores<br>RAM: ~8-12 GB | Profile for "weak segment" PCs. If your PC is not a "potato", but still fairly weak, this might work for you.                                                                        |
| `medium.json` | CPU: 6+ cores<br>RAM: ~16 GB   | Balanced profile for "mid-range segment" PCs. Suitable for most modern systems.                                                                                                        |
| `max.json`    | CPU: 8+ cores<br>RAM: ~32+ GB  | Profile for "high-performance segment" PCs. Suitable for maximum JVM performance while maintaining stable operation within STALCRAFT: X.                                              |

To use any of the presets, navigate to the repository [`/configs`](./configs/) folder,
from where you can download the profile you're interested in to the `jvm_wrapper/configs` folder.

Run the utility, select `Select Config` in the menu. Now, in addition to `default.json`, another
configuration profile should appear in the list. Select it, then restart the game.

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

### Large Pages

**Large Pages** is a virtual memory mode in which larger pages are used than the standard 4 KB.

Enabling Large Pages reduces memory access overhead, making GC and heap access more stable and faster.
This happens because the CPU doesn't directly access your RAM. The CPU uses TLB (Translation Lookaside Buffer).

> [!CAUTION]
> Large Pages lock memory to the application and prevent the system from reallocating it.
> Incorrect configuration can lead to unstable OS operation. Be aware of your actions!
> Make sure that the allocated memory in your configuration profile does not exceed 40%-50% of total RAM,
> and that you have at least 16+ GB of free RAM.

To enable Large Pages, follow these steps:

1. Press `win` + `R`.
2. Type `secpol.msc` and press `Enter`.
3. Navigate to *Local Policies → User Rights Assignment*.
4. Find the *"Lock pages in memory"* policy.
5. Open the policy by double-clicking it, add your user or the "Administrators" group.
6. Apply the changes and restart your PC.

### Technical Information

Detailed technical information describing the utility's operating principles,
as well as build instructions can be found [here](./docs/OVERVIEW.en.md).
