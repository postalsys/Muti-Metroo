---
title: System Service
sidebar_position: 4
---

# System Service Installation

Install Muti Metroo as a system service for automatic startup and management.

## Linux (systemd)

### Installation

```bash
# Install as service (requires root)
sudo ./build/muti-metroo service install -c /etc/muti-metroo/config.yaml
```

This creates a systemd unit file at `/etc/systemd/system/muti-metroo.service`.

### Service Management

```bash
# Enable automatic start on boot
sudo systemctl enable muti-metroo

# Start the service
sudo systemctl start muti-metroo

# Check status
sudo systemctl status muti-metroo

# View logs
sudo journalctl -u muti-metroo -f

# Restart
sudo systemctl restart muti-metroo

# Stop
sudo systemctl stop muti-metroo
```

### Uninstall

```bash
# Stop and disable
sudo systemctl stop muti-metroo
sudo systemctl disable muti-metroo

# Uninstall service
sudo muti-metroo service uninstall

# Clean up files (optional)
sudo rm -rf /etc/muti-metroo
sudo rm -rf /var/lib/muti-metroo
```

### Systemd Unit File

The installer creates:

```ini
# /etc/systemd/system/muti-metroo.service
[Unit]
Description=Muti Metroo Mesh Networking Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=muti-metroo
Group=muti-metroo
ExecStart=/usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/muti-metroo

[Install]
WantedBy=multi-user.target
```

### Custom Installation

For manual setup:

```bash
# Create user
sudo useradd -r -s /sbin/nologin muti-metroo

# Create directories
sudo mkdir -p /etc/muti-metroo
sudo mkdir -p /var/lib/muti-metroo
sudo chown muti-metroo:muti-metroo /var/lib/muti-metroo

# Copy binary
sudo cp ./build/muti-metroo /usr/local/bin/

# Copy config
sudo cp ./config.yaml /etc/muti-metroo/
sudo chown root:muti-metroo /etc/muti-metroo/config.yaml
sudo chmod 640 /etc/muti-metroo/config.yaml

# Copy certificates
sudo cp -r ./certs /etc/muti-metroo/
sudo chown -R root:muti-metroo /etc/muti-metroo/certs
sudo chmod 750 /etc/muti-metroo/certs
sudo chmod 640 /etc/muti-metroo/certs/*

# Create unit file
sudo nano /etc/systemd/system/muti-metroo.service
# (paste unit file content)

# Reload and start
sudo systemctl daemon-reload
sudo systemctl enable --now muti-metroo
```

### Configuration for Service

```yaml
# /etc/muti-metroo/config.yaml
agent:
  id: "auto"
  display_name: "${HOSTNAME}"
  data_dir: "/var/lib/muti-metroo"
  log_level: "info"
  log_format: "json"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "/etc/muti-metroo/certs/agent.crt"
      key: "/etc/muti-metroo/certs/agent.key"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: "127.0.0.1:8080"

control:
  enabled: true
  socket_path: "/var/lib/muti-metroo/control.sock"
```

## Windows Service

### Installation

Run as Administrator:

```powershell
# Install service
muti-metroo.exe service install -c C:\ProgramData\muti-metroo\config.yaml
```

### Service Management

```powershell
# Start
sc start muti-metroo

# Stop
sc stop muti-metroo

# Status
sc query muti-metroo

# Configure automatic start
sc config muti-metroo start= auto
```

Or use Services GUI:
1. Open `services.msc`
2. Find "Muti Metroo"
3. Right-click for Start/Stop/Properties

### Uninstall

Run as Administrator:

```powershell
# Stop service
sc stop muti-metroo

# Uninstall
muti-metroo.exe service uninstall
```

### Windows Paths

```
C:\ProgramData\muti-metroo\
  config.yaml           # Configuration
  data\                 # Agent data
    agent_id            # Agent identity
  certs\                # Certificates
    agent.crt
    agent.key
    ca.crt
C:\Program Files\muti-metroo\
  muti-metroo.exe       # Binary
```

### Configuration for Windows

```yaml
# C:\ProgramData\muti-metroo\config.yaml
agent:
  id: "auto"
  data_dir: "C:\\ProgramData\\muti-metroo\\data"
  log_level: "info"
  log_format: "json"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "C:\\ProgramData\\muti-metroo\\certs\\agent.crt"
      key: "C:\\ProgramData\\muti-metroo\\certs\\agent.key"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: "127.0.0.1:8080"
```

## macOS (launchd)

### Installation

```bash
# Copy binary
sudo cp ./build/muti-metroo /usr/local/bin/

# Create directories
sudo mkdir -p /etc/muti-metroo
sudo mkdir -p /var/lib/muti-metroo

# Copy config and certs
sudo cp ./config.yaml /etc/muti-metroo/
sudo cp -r ./certs /etc/muti-metroo/

# Create launchd plist
sudo nano /Library/LaunchDaemons/com.muti-metroo.agent.plist
```

### Launchd Plist

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muti-metroo.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/muti-metroo</string>
        <string>run</string>
        <string>-c</string>
        <string>/etc/muti-metroo/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/var/log/muti-metroo.log</string>
    <key>StandardOutPath</key>
    <string>/var/log/muti-metroo.log</string>
</dict>
</plist>
```

### Service Management

```bash
# Load and start
sudo launchctl load /Library/LaunchDaemons/com.muti-metroo.agent.plist

# Stop
sudo launchctl stop com.muti-metroo.agent

# Unload
sudo launchctl unload /Library/LaunchDaemons/com.muti-metroo.agent.plist

# View logs
tail -f /var/log/muti-metroo.log
```

## Security Considerations

### File Permissions

```bash
# Linux
sudo chmod 700 /var/lib/muti-metroo
sudo chmod 640 /etc/muti-metroo/config.yaml
sudo chmod 600 /etc/muti-metroo/certs/*.key
sudo chmod 644 /etc/muti-metroo/certs/*.crt
```

### Network Ports

If using ports below 1024, either:

1. Run as root (not recommended)
2. Use capabilities:
   ```bash
   sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/muti-metroo
   ```
3. Use ports above 1024

### Firewall Rules

```bash
# Linux (firewalld)
sudo firewall-cmd --permanent --add-port=4433/udp
sudo firewall-cmd --permanent --add-port=1080/tcp
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# Linux (ufw)
sudo ufw allow 4433/udp
sudo ufw allow 1080/tcp
sudo ufw allow 8080/tcp
```

## Troubleshooting

### Service Won't Start

```bash
# Check systemd logs
sudo journalctl -u muti-metroo -n 50

# Check service status
sudo systemctl status muti-metroo

# Test config manually
sudo -u muti-metroo /usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml
```

### Permission Denied

```bash
# Check file ownership
ls -la /etc/muti-metroo/
ls -la /var/lib/muti-metroo/

# Fix permissions
sudo chown -R muti-metroo:muti-metroo /var/lib/muti-metroo
```

### Port Already in Use

```bash
# Find what's using the port
sudo lsof -i :4433
sudo netstat -tlnp | grep 4433
```

## Next Steps

- [High Availability](high-availability) - Redundancy setup
- [Monitoring](../features/metrics-monitoring) - Set up monitoring
- [Troubleshooting](../troubleshooting/common-issues) - Common issues
