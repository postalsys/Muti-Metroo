---
title: service
---

# muti-metroo service

Service management commands for system integration.

## Subcommands

### service install

Install as system service (requires root/admin).

```bash
muti-metroo service install -c <config-file>
```

**Flags:**
- `-c, --config <file>`: Configuration file path (required)

**Linux (systemd):**
- Creates `/etc/systemd/system/muti-metroo.service`
- Reloads systemd daemon

**Windows:**
- Registers Windows Service
- Sets to automatic startup

### service uninstall

Uninstall system service.

```bash
muti-metroo service uninstall
```

Removes service registration on both Linux and Windows.

### service status

Check service status.

```bash
muti-metroo service status
```

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

# Windows install (as Administrator)
muti-metroo service install -c C:\Program Files\muti-metroo\config.yaml

# Uninstall
sudo muti-metroo service uninstall  # Linux
muti-metroo service uninstall        # Windows (as Administrator)
```
