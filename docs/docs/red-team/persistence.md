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

# macOS (launchd) - requires root
sudo muti-metroo service install -c /Library/Application\ Support/AppService/config.yaml

# Windows (Windows Service) - requires Administrator
muti-metroo.exe service install -c C:\ProgramData\AppService\config.yaml
```

**Service locations:**

| Platform | Method | Location |
|----------|--------|----------|
| Linux | systemd (root) | `/etc/systemd/system/muti-metroo.service` |
| Linux | cron+nohup (user) | `~/.muti-metroo/muti-metroo.sh` + cron `@reboot` |
| macOS | launchd | `/Library/LaunchDaemons/com.muti-metroo.plist` |
| Windows | Service Manager | Windows Service Registry |

## Non-Root Persistence (Linux)

When root access is unavailable, use the `--user` flag to install via cron+nohup:

```bash
# Install without root privileges
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
- No auto-restart on crash (unlike systemd)
- Works on any Linux with cron installed

**Comparison:**

| Feature | systemd (root) | cron+nohup (user) |
|---------|----------------|-------------------|
| Requires root | Yes | No |
| Auto-restart | Yes | No |
| Log management | journald | File-based |
| Boot persistence | Yes | Yes |
| Process visibility | `systemctl` | PID file |

Consider renaming the binary and service to blend with the environment.
