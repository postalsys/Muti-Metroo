---
title: service
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole managing services" style={{maxWidth: '180px'}} />
</div>

# muti-metroo service

Install the agent as a system service so it starts automatically on boot and restarts if it crashes. Works on Linux (systemd or cron), macOS (launchd), and Windows.

**Quick install:**
```bash
# Linux/macOS (requires root)
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Linux without root (uses cron)
muti-metroo service install --user -c ~/muti-metroo/config.yaml

# Windows (as Administrator)
muti-metroo service install -c C:\muti-metroo\config.yaml
```

## Subcommands

### service install

Install as system service.

```bash
muti-metroo service install -c <config-file> [-n <service-name>] [--user]
```

**Flags:**
- `-c, --config <file>`: Configuration file path (required)
- `-n, --name <name>`: Service name (default: muti-metroo)
- `--user`: Install as user service using cron+nohup (Linux only, no root required)

**Linux (systemd)** - requires root:
- Creates `/etc/systemd/system/muti-metroo.service`
- Reloads systemd daemon
- Enables automatic startup

**Linux (cron+nohup)** - with `--user` flag, no root required:
- Creates `~/.muti-metroo/muti-metroo.sh` wrapper script
- Adds `@reboot` cron entry
- Logs to `~/.muti-metroo/muti-metroo.log`

**macOS (launchd)** - requires root:
- Creates `/Library/LaunchDaemons/com.muti-metroo.plist`
- Loads service with `launchctl`

**Windows** - requires Administrator:
- Registers Windows Service
- Sets to automatic startup

### service uninstall

Uninstall system service.

```bash
muti-metroo service uninstall [-n <service-name>] [-f]
```

**Flags:**
- `-n, --name <name>`: Service name (default: muti-metroo)
- `-f, --force`: Skip confirmation prompt

Removes service registration. On Linux, automatically detects whether systemd or cron+nohup was used.

### service status

Check service status.

```bash
muti-metroo service status [-n <service-name>]
```

**Flags:**
- `-n, --name <name>`: Service name (default: muti-metroo)

Shows current service state (running, stopped, etc.).

## Linux: Systemd vs Cron+Nohup

| Feature | Systemd | Cron+Nohup |
|---------|---------|------------|
| Requires root | Yes | No |
| Auto-restart on crash | Yes | No |
| Log management | journald | File-based |
| Resource limits | cgroups | None |
| Start on boot | Yes | Yes |

**Use systemd when:**
- You have root access
- You need automatic restart on crash
- You want journald integration

**Use cron+nohup when:**
- You don't have root access
- You're deploying to a user account
- Systemd is unavailable

## Linux Management (Systemd)

After installation with root:

```bash
# Enable and start
sudo systemctl enable muti-metroo
sudo systemctl start muti-metroo

# Check status
sudo systemctl status muti-metroo

# View logs
sudo journalctl -u muti-metroo -f

# Restart
sudo systemctl restart muti-metroo
```

## Linux Management (Cron+Nohup)

After installation with `--user`:

```bash
# Check status
muti-metroo service status

# View logs
tail -f ~/.muti-metroo/muti-metroo.log

# Manually start (if stopped)
~/.muti-metroo/muti-metroo.sh

# Uninstall
muti-metroo service uninstall
```

## macOS Management

After installation:

```bash
# Check status
sudo launchctl list | grep muti-metroo

# Stop service
sudo launchctl stop com.muti-metroo

# Start service
sudo launchctl start com.muti-metroo

# View logs
tail -f /var/log/muti-metroo.out.log
```

## Windows Management

After installation:

```powershell
# Start service
sc start muti-metroo

# Check status
sc query muti-metroo

# Stop service
sc stop muti-metroo
```

## Examples

```bash
# Linux install (systemd, requires root)
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml
sudo systemctl enable --now muti-metroo

# Linux install (cron+nohup, no root required)
muti-metroo service install --user -c ~/muti-metroo/config.yaml

# macOS install
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Windows install (as Administrator)
muti-metroo service install -c C:\Program Files\muti-metroo\config.yaml

# Check status (all platforms)
muti-metroo service status

# Uninstall (auto-detects installation type on Linux)
sudo muti-metroo service uninstall  # Linux systemd / macOS
muti-metroo service uninstall        # Linux user service / Windows (as Admin)
```
