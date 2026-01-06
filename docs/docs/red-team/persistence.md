---
title: Persistence
sidebar_label: Persistence
sidebar_position: 7
---

# Persistence

Methods for maintaining persistent access across system reboots.

## System Service Installation

Install as a system service for persistence:

```bash
# Linux (systemd) - requires root
sudo muti-metroo service install -c /etc/app-service/config.yaml

# Linux (cron+nohup) - NO root required
muti-metroo service install --user -c ~/app-service/config.yaml

# macOS (launchd daemon) - requires root
sudo muti-metroo service install -c /Library/Application\ Support/AppService/config.yaml

# macOS (launchd agent) - NO root required
muti-metroo service install --user -c ~/Library/Application\ Support/AppService/config.yaml

# Windows (Windows Service) - requires Administrator
muti-metroo.exe service install -c C:\ProgramData\AppService\config.yaml
```

## Service Names and Locations

Default service identifiers created by installation:

| Platform | Method | Service Name | Location |
|----------|--------|--------------|----------|
| Linux | systemd | `muti-metroo.service` | `/etc/systemd/system/muti-metroo.service` |
| Linux | cron (user) | cron `@reboot` entry | `~/.muti-metroo/muti-metroo.sh` |
| macOS | launchd daemon | `com.muti-metroo` | `/Library/LaunchDaemons/com.muti-metroo.plist` |
| macOS | launchd agent | `com.muti-metroo` | `~/Library/LaunchAgents/com.muti-metroo.plist` |
| Windows | Service Manager | `MutiMetroo` | `HKLM\SYSTEM\CurrentControlSet\Services\MutiMetroo` |

:::tip Operational Naming
For operational deployments, rename the binary before installation. The service name is derived from the binary name:

```bash
# Rename binary to blend with environment
cp muti-metroo /usr/local/bin/app-health-monitor

# Service will be named "app-health-monitor.service" on Linux
sudo /usr/local/bin/app-health-monitor service install -c /etc/app-health/config.yaml
```

On Windows, consider names like `WindowsHealthService`, `SystemCacheManager`, or `ApplicationPoolWorker`.
:::

## Linux Persistence

### Root: systemd

```bash
# Install
sudo muti-metroo service install -c /etc/app-service/config.yaml

# Manage
sudo systemctl status muti-metroo
sudo systemctl start muti-metroo
sudo systemctl stop muti-metroo
sudo systemctl restart muti-metroo

# View logs
sudo journalctl -u muti-metroo -f

# Uninstall
sudo muti-metroo service uninstall
```

**systemd unit characteristics:**
- Auto-restart on crash (`Restart=always`)
- Starts after network is available
- Logs to journald (can be forwarded to syslog)
- Process visible via `systemctl list-units`

### User: cron+nohup

When root access is unavailable:

```bash
# Install
muti-metroo service install --user -c ~/config.yaml

# Check status
muti-metroo service status

# View logs
tail -f ~/.muti-metroo/muti-metroo.log

# Uninstall
muti-metroo service uninstall
```

**Cron+nohup characteristics:**
- Creates `@reboot` cron entry for automatic startup
- Logs to `~/.muti-metroo/muti-metroo.log`
- PID file at `~/.muti-metroo/muti-metroo.pid`
- No auto-restart on crash
- Works on any Linux with cron installed

## macOS Persistence

### Root: LaunchDaemon

System-wide daemon that runs regardless of user login:

```bash
# Install (requires root)
sudo muti-metroo service install -c /Library/Application\ Support/AppService/config.yaml

# Manage
sudo launchctl list | grep muti-metroo
sudo launchctl start com.muti-metroo
sudo launchctl stop com.muti-metroo

# View logs
tail -f /var/log/muti-metroo.out.log
tail -f /var/log/muti-metroo.err.log

# Uninstall
sudo muti-metroo service uninstall
```

**LaunchDaemon characteristics:**
- Runs at system boot, before user login
- Runs as root
- Plist at `/Library/LaunchDaemons/com.muti-metroo.plist`
- Auto-restart via `KeepAlive` directive
- Logs to `/var/log/muti-metroo.{out,err}.log`

### User: LaunchAgent

User-level agent that runs when the user logs in (no root required):

```bash
# Install (no root needed)
muti-metroo service install --user -c ~/Library/Application\ Support/AppService/config.yaml

# Manage
launchctl list | grep muti-metroo
launchctl start com.muti-metroo
launchctl stop com.muti-metroo

# View logs
tail -f ~/Library/Logs/muti-metroo.out.log
tail -f ~/Library/Logs/muti-metroo.err.log

# Uninstall
muti-metroo service uninstall
```

**LaunchAgent characteristics:**
- Runs when user logs in (GUI or SSH)
- Runs as the installing user
- Plist at `~/Library/LaunchAgents/com.muti-metroo.plist`
- Auto-restart via `KeepAlive` directive
- Logs to `~/Library/Logs/muti-metroo.{out,err}.log`

## Windows Persistence

### Administrator: Windows Service

```powershell
# Install (requires Administrator)
muti-metroo.exe service install -c C:\ProgramData\AppService\config.yaml

# Manage via PowerShell
Get-Service MutiMetroo
Start-Service MutiMetroo
Stop-Service MutiMetroo
Restart-Service MutiMetroo

# Or via sc.exe
sc query MutiMetroo
sc start MutiMetroo
sc stop MutiMetroo

# View in Event Viewer
# Application and Services Logs > Application

# Uninstall
muti-metroo.exe service uninstall
```

**Windows Service characteristics:**
- Runs at system boot, before user login
- Runs as SYSTEM account
- Visible in `services.msc`
- Auto-restart configurable via service recovery options
- Registry key at `HKLM\SYSTEM\CurrentControlSet\Services\<ServiceName>`

### User: Scheduled Task (Manual)

For user-level persistence without Administrator:

```powershell
# Create scheduled task to run at logon
$Action = New-ScheduledTaskAction -Execute "C:\Users\$env:USERNAME\AppData\Local\app-service\app-service.exe" `
  -Argument "run -c C:\Users\$env:USERNAME\AppData\Local\app-service\config.yaml"
$Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
Register-ScheduledTask -TaskName "AppServiceMonitor" -Action $Action -Trigger $Trigger -Settings $Settings

# Check status
Get-ScheduledTask -TaskName "AppServiceMonitor"

# Run manually
Start-ScheduledTask -TaskName "AppServiceMonitor"

# Remove
Unregister-ScheduledTask -TaskName "AppServiceMonitor" -Confirm:$false
```

### User: Run Registry Key (Manual)

Alternative user-level persistence:

```powershell
# Add to current user's Run key
$RegPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$ExePath = "C:\Users\$env:USERNAME\AppData\Local\app-service\app-service.exe"
$Args = "run -c C:\Users\$env:USERNAME\AppData\Local\app-service\config.yaml"
Set-ItemProperty -Path $RegPath -Name "AppServiceMonitor" -Value "$ExePath $Args"

# Verify
Get-ItemProperty -Path $RegPath -Name "AppServiceMonitor"

# Remove
Remove-ItemProperty -Path $RegPath -Name "AppServiceMonitor"
```

## Comparison Matrix

| Feature | Linux systemd | Linux cron | macOS Daemon | macOS Agent | Windows Service | Windows Task |
|---------|---------------|------------|--------------|-------------|-----------------|--------------|
| Requires root/admin | Yes | No | Yes | No | Yes | No |
| Runs at boot | Yes | Yes | Yes | No (login) | Yes | No (login) |
| Auto-restart | Yes | No | Yes | Yes | Yes | No |
| Process visibility | systemctl | ps | launchctl | launchctl | services.msc | Task Scheduler |
| Log management | journald | File | File | File | Event Log | None |

## Blending Considerations

### Naming Conventions

Choose service names that match the target environment:

| Environment | Suggested Names |
|-------------|-----------------|
| Web server | `nginx-cache-worker`, `apache-mod-proxy` |
| Database | `mysql-replication-agent`, `pg-backup-service` |
| Monitoring | `prometheus-node-exporter`, `datadog-agent-helper` |
| Cloud | `aws-ssm-agent-helper`, `gcp-guest-agent` |
| Generic | `system-health-monitor`, `application-pool-worker` |

### File Locations

Place binaries and configs in locations that match the cover story:

| Platform | Legitimate-looking Paths |
|----------|-------------------------|
| Linux | `/usr/local/bin/`, `/opt/<app>/`, `/var/lib/<app>/` |
| macOS | `/usr/local/bin/`, `/Library/Application Support/<app>/` |
| Windows | `C:\Program Files\<Company>\<App>\`, `C:\ProgramData\<App>\` |

### Avoiding Detection

- Match file timestamps to surrounding files: `touch -r /bin/ls /usr/local/bin/myagent`
- Use appropriate file permissions (not 777)
- Ensure log files don't grow unbounded
- Consider log rotation or `log_level: error`
