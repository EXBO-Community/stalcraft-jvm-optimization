#Requires -Version 5.1
[CmdletBinding()]
param(
    # Internal retry marker. This does not grant administrator rights.
    [switch]$Elevated
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    [Console]::Error.WriteLine('uninstall.ps1 requires Windows. No changes were made.')
    exit 1
}

function Test-WrapperDebugger([string]$Command) {
    $path = $Command.Trim()
    $quoted = $path.StartsWith('"', [StringComparison]::Ordinal)
    if ($quoted) {
        if ($path.Length -lt 2 -or -not $path.EndsWith('"', [StringComparison]::Ordinal)) {
            return $false
        }
        $path = $path.Substring(1, $path.Length - 2)
    }
    $ordinaryPath = $path
    if ($path.StartsWith('\\?\', [StringComparison]::Ordinal) -or $path.StartsWith('\\.\', [StringComparison]::Ordinal)) {
        $ordinaryPath = $path.Substring(4)
    }
    $invalid = [char[]]@([char]34, [char]60, [char]62, [char]124, [char]42, [char]63, [char]0, [char]13, [char]10, [char]9)
    if ($ordinaryPath.IndexOfAny($invalid) -ge 0) {
        return $false
    }

    $separator = $path.LastIndexOfAny([char[]]@('\', '/'))
    $name = $path.Substring($separator + 1)
    if (-not $name.Equals('service.exe', [StringComparison]::OrdinalIgnoreCase) -and
        -not $name.Equals('wrapper.exe', [StringComparison]::OrdinalIgnoreCase)) {
        return $false
    }
    if ($separator -lt 0) {
        return $true
    }

    $colon = $ordinaryPath.IndexOf(':')
    if ($colon -ge 0) {
        if ($colon -ne 1 -or $ordinaryPath.Length -lt 3 -or $ordinaryPath.IndexOf(':', 2) -ge 0) {
            return $false
        }
        $drive = [int][char]$ordinaryPath[0]
        if (($drive -lt 65 -or $drive -gt 90) -and ($drive -lt 97 -or $drive -gt 122)) {
            return $false
        }
        if ($ordinaryPath[2] -ne '\' -and $ordinaryPath[2] -ne '/') {
            return $false
        }
    } elseif (-not $path.StartsWith('\\', [StringComparison]::Ordinal) -and
              -not $path.StartsWith('//', [StringComparison]::Ordinal)) {
        if (-not $path.StartsWith('%', [StringComparison]::Ordinal)) {
            return $false
        }
        $end = $path.IndexOf('%', 1)
        if ($end -le 1 -or $end + 1 -ge $path.Length -or ($path[$end + 1] -ne '\' -and $path[$end + 1] -ne '/')) {
            return $false
        }
    }

    # Early installers wrote unquoted paths, including paths with spaces.
    # Refuse ambiguous executable-plus-arguments commands.
    if (-not $quoted) {
        for ($i = 0; $i -lt $path.Length; $i++) {
            if ($i + 4 -lt $path.Length -and $path.Substring($i, 4).Equals('.exe', [StringComparison]::OrdinalIgnoreCase) -and $path[$i + 4] -eq ' ') {
                return $false
            }
            if ($path[$i] -eq ' ' -and $i + 1 -lt $path.Length -and ($path[$i + 1] -eq '\' -or $path[$i + 1] -eq '/')) {
                return $false
            }
        }
    }
    return $true
}

function Test-AccessDenied([Exception]$Exception) {
    while ($null -ne $Exception) {
        if ($Exception -is [UnauthorizedAccessException] -or $Exception -is [Security.SecurityException]) {
            return $true
        }
        if ($Exception -is [ComponentModel.Win32Exception] -and $Exception.NativeErrorCode -eq 5) {
            return $true
        }
        $Exception = $Exception.InnerException
    }
    return $false
}

$ifeoPath = 'SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options'
$targets = @('stalzone.exe', 'stalzonew.exe', 'stalcraft.exe', 'stalcraftw.exe')
$view = [Microsoft.Win32.RegistryView]::Registry32
if ([Environment]::Is64BitOperatingSystem) {
    $view = [Microsoft.Win32.RegistryView]::Registry64
}
$readRights = [Security.AccessControl.RegistryRights]::QueryValues
$writeRights = $readRights -bor [Security.AccessControl.RegistryRights]::SetValue
$valueOptions = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames

[Console]::WriteLine("Recovery scope: native HKLM\$ifeoPath on stalzone.exe/stalzonew.exe and legacy stalcraft.exe/stalcraftw.exe; wrapper-owned Debugger values only.")
$failed = $false
$denied = $false
$base = $null
try {
    $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine, $view)
    foreach ($target in $targets) {
        $path = "$ifeoPath\$target"
        $readKey = $null
        $writeKey = $null
        try {
            $readKey = $base.OpenSubKey($path, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, $readRights)
            if ($null -eq $readKey) {
                [Console]::WriteLine("${target}: no IFEO key; nothing to remove.")
                continue
            }
            $debugger = $readKey.GetValue('Debugger', $null, $valueOptions)
            if ($null -eq $debugger) {
                [Console]::WriteLine("${target}: no Debugger value; nothing to remove.")
                continue
            }
            $kind = $readKey.GetValueKind('Debugger')
            if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) {
                throw [InvalidOperationException]::new('Debugger has a non-string registry type; left unchanged.')
            }
            if (-not (Test-WrapperDebugger ([string]$debugger))) {
                throw [InvalidOperationException]::new('Debugger is not a recognized service.exe or historical wrapper.exe path-only hook; left unchanged.')
            }

            $writeKey = $base.OpenSubKey($path, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, $writeRights)
            if ($null -eq $writeKey) {
                [Console]::WriteLine("${target}: IFEO key was already removed.")
                continue
            }
            $current = $writeKey.GetValue('Debugger', $null, $valueOptions)
            if ($null -eq $current) {
                [Console]::WriteLine("${target}: Debugger was already removed.")
                continue
            }
            if ($writeKey.GetValueKind('Debugger') -ne $kind -or -not [string]::Equals([string]$current, [string]$debugger, [StringComparison]::Ordinal)) {
                throw [InvalidOperationException]::new('Debugger changed during recovery; left unchanged.')
            }
            $writeKey.DeleteValue('Debugger', $false)
            [Console]::WriteLine("${target}: removed wrapper Debugger; other values, subkeys and permissions preserved.")
        } catch {
            $failed = $true
            if (Test-AccessDenied $_.Exception) {
                $denied = $true
            }
            [Console]::Error.WriteLine("Cannot uninstall HKLM\${path}: $($_.Exception.Message)")
        } finally {
            if ($null -ne $writeKey) { $writeKey.Dispose() }
            if ($null -ne $readKey) { $readKey.Dispose() }
        }
    }
} catch {
    $failed = $true
    if (Test-AccessDenied $_.Exception) {
        $denied = $true
    }
    [Console]::Error.WriteLine("Cannot access native HKLM\${ifeoPath}: $($_.Exception.Message)")
} finally {
    if ($null -ne $base) { $base.Dispose() }
}

if ($denied -and -not $Elevated) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $principal = [Security.Principal.WindowsPrincipal]::new($identity)
        $isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } finally {
        $identity.Dispose()
    }
    if (-not $isAdmin) {
        [Console]::WriteLine('Administrator rights are needed for the remaining hook(s). Requesting UAC elevation once.')
        try {
            $hostPath = (Get-Process -Id $PID).Path
            $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $PSCommandPath + '"'), '-Elevated')
            $child = Start-Process -FilePath $hostPath -ArgumentList $arguments -Verb RunAs -Wait -PassThru
            if ($null -eq $child.ExitCode) {
                throw [InvalidOperationException]::new('The elevated process did not report an exit code.')
            }
            if ($child.ExitCode -ne 0) {
                [Console]::Error.WriteLine('Elevated recovery failed. The hook(s) reported by that process remain unresolved.')
            }
            exit $child.ExitCode
        } catch {
            [Console]::Error.WriteLine("Could not complete UAC recovery (including a cancelled prompt): $($_.Exception.Message)")
            exit 1
        }
    }
}

if ($failed) {
    [Console]::Error.WriteLine('Recovery did not complete for every target. No foreign Debugger, other registry data or permissions were removed.')
    exit 1
}
[Console]::WriteLine('Recovery complete. The wrapper files are no longer required to start these game targets.')
exit 0
