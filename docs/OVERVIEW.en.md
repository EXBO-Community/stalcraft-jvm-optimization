# Technical Information

## Operating Mechanism

The wrapper uses the IFEO (Image File Execution Options) mechanism to intercept game startup.
When `stalcraft.exe` or `stalcraftw.exe` is launched, the call is redirected to the wrapper. Its behavior:

1. Loads the active configuration file from the `configs/` directory.
2. Removes conflicting flags from the arguments of the original launcher.
3. Creates a process directly through `ntdll!NtCreateUserProcess` to avoid re-interception through IFEO.
4. Elevates memory and I/O priorities using `NtSetInformationProcess`.
5. Terminates after the game process displays its first visible window.

## CLI Interaction

Installing IFEO interception

```bash
wrapper.exe --install     # install IFEO interception
```

Checking interception status

```bash
wrapper.exe --status      # check interception status
```

Removing IFEO interception

```bash
wrapper.exe --uninstall   # remove IFEO interception
```

## Building the Project

`wrapper.exe` can not only be downloaded from releases, but also built yourself.
To do this, simply run the command:

```bash
cd wrapper & go build -o wrapper.exe -ldflags="-s -w" .
```
