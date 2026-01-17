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

# Windows without admin (uses Registry Run key + DLL)
muti-metroo service install --user --dll C:\path\to\muti-metroo.dll -c C:\muti-metroo\config.yaml
```

## Subcommands

### service install

Install as system service.

```bash
muti-metroo service install -c <config-file> [-n <service-name>] [--user] [--dll <dll-path>]
```

**Flags:**
- `-c, --config <file>`: Configuration file path (required)
- `-n, --name <name>`: Service name (default: muti-metroo)
- `--user`: Install as user service (Linux: cron+nohup, Windows: Registry Run)
- `--dll <path>`: Path to muti-metroo.dll (Windows `--user` mode only)

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

**Windows Service** - requires Administrator:
- Registers Windows Service
- Sets to automatic startup

**Windows Registry Run** - with `--user --dll` flags, no admin required:
- Adds entry to `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
- Runs at user logon via `rundll32.exe`
- No console window (background execution)
- The `-n` flag sets the Registry value name, converted to PascalCase (e.g., "My Tunnel" becomes "MyTunnel")

:::tip Auto-Start
All installation methods start the service immediately after installation. You don't need to manually start it or reboot.
:::

### service uninstall

Uninstall system service.

```bash
muti-metroo service uninstall [-n <service-name>] [-f]
```

**Flags:**
- `-n, --name <name>`: Service name (default: muti-metroo)
- `-f, --force`: Skip confirmation prompt

Removes service registration. On Linux and Windows, automatically detects whether system service or user service was used.

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

## Windows: Windows Service vs Registry Run

| Feature | Windows Service | Registry Run |
|---------|----------------|--------------|
| Requires admin | Yes | No |
| Auto-restart on crash | Yes | No |
| Start on boot | Yes (before login) | Yes (at login) |
| Console window | No | No |
| Runs as | SYSTEM/service account | Current user |
| Process name | `muti-metroo.exe` | `rundll32.exe` |
| Requires DLL | No | Yes |

**Use Windows Service when:**
- You have Administrator access
- You need automatic restart on crash
- Service must start before user login

**Use Registry Run when:**
- You don't have Administrator access
- User-level background execution is sufficient
- You want minimal installation footprint

## Linux Management (Systemd)

After installation with root (service auto-starts):

```bash
# Check status
sudo systemctl status muti-metroo

# View logs
sudo journalctl -u muti-metroo -f

# Restart (after config changes)
sudo systemctl restart muti-metroo

# Stop
sudo systemctl stop muti-metroo
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

## Windows Management (Windows Service)

After installation as Administrator:

```powershell
# Start service
sc start muti-metroo

# Check status
sc query muti-metroo

# Stop service
sc stop muti-metroo
```

## Windows Management (Registry Run)

After installation with `--user --dll`:

```powershell
# Check status (shows DLL and config paths)
muti-metroo service status

# Uninstall
muti-metroo service uninstall

# View registry entry (default name)
reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v MutiMetroo

# View registry entry (custom name, e.g., -n "My Tunnel" becomes "MyTunnel")
reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v MyTunnel

# Stop manually (if needed)
taskkill /F /IM rundll32.exe
```

:::tip Registry Value Names
The `-n` flag sets the service name, which is converted to PascalCase for the registry value:
- `muti-metroo` (default) becomes `MutiMetroo`
- `my-tunnel` becomes `MyTunnel`
- `My Tunnel` becomes `MyTunnel`
:::

:::note
The service starts automatically after installation and at each user logon. To restart the service, uninstall and reinstall it, or log out and log back in.
:::

## Examples

```bash
# Linux install (systemd, requires root) - auto-starts
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Linux install (cron+nohup, no root required) - auto-starts
muti-metroo service install --user -c ~/muti-metroo/config.yaml

# macOS install - auto-starts
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Windows install (as Administrator) - auto-starts
muti-metroo service install -c C:\Program Files\muti-metroo\config.yaml

# Windows install (no admin required, uses DLL) - auto-starts
muti-metroo service install --user --dll C:\path\to\muti-metroo.dll -c C:\path\to\config.yaml

# Windows install with custom name (no admin required)
muti-metroo service install --user -n "My Tunnel" --dll C:\path\to\muti-metroo.dll -c C:\path\to\config.yaml

# Check status (all platforms)
muti-metroo service status

# Uninstall (auto-detects installation type)
sudo muti-metroo service uninstall  # Linux systemd / macOS
muti-metroo service uninstall        # Linux user service / Windows user service
```
