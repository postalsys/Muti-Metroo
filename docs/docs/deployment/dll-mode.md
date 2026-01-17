---
title: DLL Mode (Windows)
sidebar_position: 6
description: Running Muti Metroo as a Windows DLL via rundll32.exe
---

# DLL Mode (Windows)

Run Muti Metroo as a DLL via `rundll32.exe` for background execution without a console window.

## Process Behavior

When launched via `rundll32.exe`, the agent runs as a **background process** - similar to a service but without being a real Windows service:

- **No console window**: The process runs silently without any visible window
- **No taskbar presence**: Does not appear in the taskbar
- **Visible in Task Manager**: Appears as `rundll32.exe` in the process list
- **No automatic restart**: Does not survive system reboots (use Registry Run for persistence)
- **No service management**: Cannot be controlled via `sc.exe` or Services console

This makes DLL mode ideal for scenarios where you need background execution without the overhead of a full Windows service installation, but remember that you must configure persistence separately if needed.

**When to use DLL mode:**
- Background execution without visible console window
- Quick deployments without service installation
- Scenarios where hiding the console is important

**When to use .exe instead:**
- Normal operation with console output
- Persistent service installation via `muti-metroo service install`
- Interactive usage and debugging

## Download

The DLL build is available on the [Download page](/download):

- **x86_64**: `muti-metroo.dll` (Windows amd64)

ARM64 Windows users can run the x64 DLL via the emulation layer (see [ARM64 Compatibility](#arm64-compatibility) below).

## Basic Usage

The DLL exports a `Run` function that can be invoked via `rundll32.exe`:

```powershell
rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml
```

The DLL runs silently in the background without opening a console window.

## ARM64 Compatibility

The DLL is built for x64 (amd64) architecture. On ARM64 Windows, use the x64 emulation layer's `rundll32.exe`:

```powershell
# On ARM64 Windows, use SysWOW64 rundll32 for x64 DLLs
C:\Windows\SysWOW64\rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml
```

The native ARM64 `rundll32.exe` (in `System32`) cannot load x64 DLLs directly.

## Configuration

The DLL requires an external configuration file - embedded configuration is not supported for DLLs due to incompatibility with UPX compression.

For single-file deployment, use the standalone `.exe` with [Embedded Configuration](/deployment/embedded-config) instead.

## Termination

Since the DLL runs without a console, there's no way to send Ctrl+C or signals for graceful shutdown. Terminate the process externally:

```powershell
# Terminate all rundll32 processes running the DLL
taskkill /F /IM rundll32.exe

# Or find and kill the specific process
Get-Process rundll32 | Where-Object { $_.MainModule.FileName -like "*muti-metroo*" } | Stop-Process -Force
```

The agent handles abrupt termination gracefully - peer connections will timeout and reconnect when the agent restarts.

## Persistence with Registry Run

Since the DLL process does not survive reboots on its own, use the Registry Run key to start it automatically at user logon. No admin privileges are required.

### Automated Installation (Recommended)

The easiest way to set up persistence is using the CLI:

```powershell
# Install as user service (no admin required)
muti-metroo service install --user --dll C:\path\to\muti-metroo.dll -c C:\path\to\config.yaml

# Install with custom service name
muti-metroo service install --user -n "My Tunnel" --dll C:\path\to\muti-metroo.dll -c C:\path\to\config.yaml
```

**Flags:**
- `--user`: Install as user service (required for Registry Run mode)
- `--dll <path>`: Path to muti-metroo.dll (required)
- `-c, --config <path>`: Path to config file (required)
- `-n, --name <name>`: Custom service name (default: muti-metroo). The name is converted to PascalCase for the Registry value (e.g., "My Tunnel" becomes "MyTunnel").

This creates a Registry Run entry at `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` that:
- Starts immediately after installation (no reboot required)
- Runs automatically at each user logon
- Uses `rundll32.exe` with the DLL
- Runs with current user privileges
- No console window (background execution)

**Management commands:**

```powershell
# Check status
muti-metroo service status

# Uninstall
muti-metroo service uninstall

# View registry entry
reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v MutiMetroo

# Stop manually (if needed)
taskkill /F /IM rundll32.exe
```

:::note
To restart the service, uninstall and reinstall it, or log out and log back in.
:::

### Manual Setup

Alternatively, create the registry entry manually:

```powershell
# Add Registry Run entry (no admin required)
$dll = "C:\Users\$env:USERNAME\muti-metroo\muti-metroo.dll"
$cfg = "C:\Users\$env:USERNAME\muti-metroo\config.yaml"
$cmd = "rundll32.exe `"$dll`",Run `"$cfg`""

Set-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" `
    -Name "MutiMetroo" -Value $cmd

# Verify
Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "MutiMetroo"
```

### Admin: Windows Service (Alternative)

For system-wide deployment that starts before any user logs in, use the Windows Service approach instead (requires Administrator):

```powershell
# Install as Windows Service (requires admin)
muti-metroo.exe service install -c C:\ProgramData\muti-metroo\config.yaml
```

Windows Service runs as SYSTEM with elevated privileges, starting at boot before user login.

## Comparison: EXE vs DLL

| Feature | .exe | .dll |
|---------|------|------|
| Console window | Yes (visible) | No (hidden) |
| Signal handling (Ctrl+C) | Yes | No |
| Service installation | Yes | No |
| Survives reboot | Yes (as service) | No (needs Registry Run) |
| Background execution | Requires `start /b` | Native |
| Embedded config | Yes | No (use config file) |
| Interactive commands | Yes | No |
| Graceful shutdown | Yes (via signals) | No (taskkill only) |
| Process name in Task Manager | `muti-metroo.exe` | `rundll32.exe` |

:::note Not a Windows Service
The DLL runs as a regular background process, not a Windows service. This means:
- It won't automatically restart after a crash
- It won't start automatically after reboot (unless configured via Registry Run)
- It cannot be managed through the Services console (`services.msc`)

For true service behavior with automatic restart and boot persistence, use `muti-metroo.exe service install` instead.
:::

## Limitations

- **Not a real service**: Runs as a background process, not a Windows service
- **No boot persistence**: Does not automatically start after reboot (use Registry Run)
- **No automatic restart**: Will not restart after a crash (unlike a Windows service)
- **No graceful shutdown**: The DLL cannot receive Windows signals for graceful termination
- **No embedded config**: Config embedding is incompatible with UPX compression; use a config file
- **No service installation**: Use the .exe for `muti-metroo service install`
- **No interactive commands**: Commands like `shell`, `upload`, `download` must use the .exe
- **No console output**: Logs must be configured to file output for debugging

## Logging Configuration

Since there's no console output, configure file-based logging in your config:

```yaml
agent:
  log_format: json
  log_file: C:\ProgramData\muti-metroo\logs\agent.log
```

## Security Considerations

- The DLL runs with the same privileges as the `rundll32.exe` process
- When running as SYSTEM via Windows Service, the agent has elevated privileges
- Consider running under a dedicated service account with minimal permissions
- The DLL is subject to the same Windows code signing requirements as .exe files

## Next Steps

- [Embedded Configuration](/deployment/embedded-config) - Single-file deployment (use .exe for this)
- [System Service](/deployment/system-service) - Native Windows Service installation
- [Download](/download) - Get the DLL binary
