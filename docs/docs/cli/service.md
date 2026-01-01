---
title: service
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole managing services" style={{maxWidth: '180px'}} />
</div>

# muti-metroo service

Service management commands for system integration.

## Subcommands

### service install

Install as system service (requires root/admin).

```bash
muti-metroo service install -c <config-file> [-n <service-name>]
```

**Flags:**
- `-c, --config <file>`: Configuration file path (required)
- `-n, --name <name>`: Service name (default: muti-metroo)

**Linux (systemd):**
- Creates `/etc/systemd/system/muti-metroo.service`
- Reloads systemd daemon

**macOS (launchd):**
- Creates `/Library/LaunchDaemons/com.muti-metroo.plist`
- Loads service with `launchctl`

**Windows:**
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

Removes service registration on Linux, macOS, and Windows.

### service status

Check service status.

```bash
muti-metroo service status [-n <service-name>]
```

**Flags:**
- `-n, --name <name>`: Service name (default: muti-metroo)

Shows current service state (running, stopped, etc.).

## Linux Management

After installation:

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
# Linux install
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml
sudo systemctl enable --now muti-metroo

# macOS install
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Windows install (as Administrator)
muti-metroo service install -c C:\Program Files\muti-metroo\config.yaml

# Check status (all platforms)
muti-metroo service status

# Uninstall
sudo muti-metroo service uninstall  # Linux/macOS
muti-metroo service uninstall        # Windows (as Administrator)
```
