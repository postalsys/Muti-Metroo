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
- **No automatic restart**: Does not survive system reboots (use Task Scheduler for persistence)
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

DLL builds are available alongside the standard .exe files on the [Download page](/download):

- **x86_64**: `muti-metroo.dll` (Windows amd64)
- **ARM64**: `muti-metroo.dll` (Windows arm64)

## Basic Usage

The DLL exports a `Run` function that can be invoked via `rundll32.exe`:

```powershell
# Run with configuration file
rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml

# Run with embedded configuration (no config file needed)
rundll32.exe C:\path\to\muti-metroo.dll,Run
```

The DLL runs silently in the background without opening a console window.

## ARM64 Compatibility

The DLL is built for x64 (amd64) architecture. On ARM64 Windows, use the x64 emulation layer's `rundll32.exe`:

```powershell
# On ARM64 Windows, use SysWOW64 rundll32 for x64 DLLs
C:\Windows\SysWOW64\rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml
```

The native ARM64 `rundll32.exe` (in `System32`) cannot load x64 DLLs directly.

## Embedded Configuration

For true single-file deployment, embed the configuration into the DLL using the setup wizard:

```powershell
# Embed config into the DLL
muti-metroo.exe setup -c C:\path\to\muti-metroo.dll
```

When prompted for "Configuration Delivery", choose **"Embed in binary"**.

After embedding, run without specifying a config file:

```powershell
rundll32.exe C:\path\to\my-agent.dll,Run
```

See [Embedded Configuration](/deployment/embedded-config) for detailed instructions on the embedding process.

## Termination

Since the DLL runs without a console, there's no way to send Ctrl+C or signals for graceful shutdown. Terminate the process externally:

```powershell
# Terminate all rundll32 processes running the DLL
taskkill /F /IM rundll32.exe

# Or find and kill the specific process
Get-Process rundll32 | Where-Object { $_.MainModule.FileName -like "*muti-metroo*" } | Stop-Process -Force
```

The agent handles abrupt termination gracefully - peer connections will timeout and reconnect when the agent restarts.

## Persistence with Task Scheduler

Since the DLL process does not survive reboots on its own, use Task Scheduler to start it automatically at system startup:

```powershell
# Create a scheduled task that runs at startup
$action = New-ScheduledTaskAction -Execute "rundll32.exe" `
    -Argument "C:\ProgramData\muti-metroo\muti-metroo.dll,Run C:\ProgramData\muti-metroo\config.yaml"

$trigger = New-ScheduledTaskTrigger -AtStartup

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask -TaskName "MutiMetroo" `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -RunLevel Highest `
    -User "SYSTEM"
```

## Comparison: EXE vs DLL

| Feature | .exe | .dll |
|---------|------|------|
| Console window | Yes (visible) | No (hidden) |
| Signal handling (Ctrl+C) | Yes | No |
| Service installation | Yes | No |
| Survives reboot | Yes (as service) | No (needs Task Scheduler) |
| Background execution | Requires `start /b` | Native |
| Embedded config | Yes | Yes |
| Interactive commands | Yes | No |
| Graceful shutdown | Yes (via signals) | No (taskkill only) |
| Process name in Task Manager | `muti-metroo.exe` | `rundll32.exe` |

:::note Not a Windows Service
The DLL runs as a regular background process, not a Windows service. This means:
- It won't automatically restart after a crash
- It won't start automatically after reboot (unless configured via Task Scheduler)
- It cannot be managed through the Services console (`services.msc`)

For true service behavior with automatic restart and boot persistence, use `muti-metroo.exe service install` instead.
:::

## Limitations

- **Not a real service**: Runs as a background process, not a Windows service
- **No boot persistence**: Does not automatically start after reboot (use Task Scheduler)
- **No automatic restart**: Will not restart after a crash (unlike a Windows service)
- **No graceful shutdown**: The DLL cannot receive Windows signals for graceful termination
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
- When running as SYSTEM via Task Scheduler, the agent has elevated privileges
- Consider running under a dedicated service account with minimal permissions
- The DLL is subject to the same Windows code signing requirements as .exe files

## Next Steps

- [Embedded Configuration](/deployment/embedded-config) - Single-file deployment
- [System Service](/deployment/system-service) - Native Windows Service installation
- [Download](/download) - Get the DLL binary
