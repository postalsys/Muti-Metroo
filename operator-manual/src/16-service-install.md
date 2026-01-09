# Service Installation

Install Muti Metroo as a system service for automatic startup and management.

## Linux (systemd)

### Installation

```bash
# Install as service (requires root)
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml
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

### Manual Setup

For custom installations:

```bash
# Create user
sudo useradd -r -s /sbin/nologin muti-metroo

# Create directories
sudo mkdir -p /etc/muti-metroo
sudo mkdir -p /var/lib/muti-metroo
sudo chown muti-metroo:muti-metroo /var/lib/muti-metroo

# Copy binary
sudo cp muti-metroo /usr/local/bin/

# Copy config and certs
sudo cp ./config.yaml /etc/muti-metroo/
sudo cp -r ./certs /etc/muti-metroo/
sudo chown -R root:muti-metroo /etc/muti-metroo
sudo chmod 640 /etc/muti-metroo/config.yaml
sudo chmod 600 /etc/muti-metroo/certs/*.key

# Create unit file
sudo tee /etc/systemd/system/muti-metroo.service << 'EOF'
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

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/muti-metroo

[Install]
WantedBy=multi-user.target
EOF

# Reload and start
sudo systemctl daemon-reload
sudo systemctl enable --now muti-metroo
```

## Windows Service

### Installation

Run as Administrator:

```powershell
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

Or use Services GUI (`services.msc`):
1. Find "Muti Metroo"
2. Right-click for Start/Stop/Properties

### Uninstall

Run as Administrator:

```powershell
sc stop muti-metroo
muti-metroo.exe service uninstall
```

### Windows Paths

```
C:\ProgramData\muti-metroo\
  config.yaml           # Configuration
  data\                 # Agent data
    agent_id
  certs\                # Certificates
    agent.crt
    agent.key
    ca.crt
C:\Program Files\muti-metroo\
  muti-metroo.exe       # Binary
```

## macOS (launchd)

### Installation

```bash
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml
```

Creates plist at `/Library/LaunchDaemons/com.muti-metroo.plist`.

### Service Management

```bash
# Check status
muti-metroo service status

# Stop
sudo launchctl stop com.muti-metroo

# Start
sudo launchctl start com.muti-metroo

# View logs
tail -f /var/log/muti-metroo.out.log
tail -f /var/log/muti-metroo.err.log
```

### Uninstall

```bash
sudo muti-metroo service uninstall
sudo rm -rf /etc/muti-metroo
sudo rm -rf /var/lib/muti-metroo
```

## Security Considerations

### File Permissions

```bash
# Linux/macOS
sudo chmod 700 /var/lib/muti-metroo
sudo chmod 640 /etc/muti-metroo/config.yaml
sudo chmod 600 /etc/muti-metroo/certs/*.key
sudo chmod 644 /etc/muti-metroo/certs/*.crt
```

### Network Ports

For ports below 1024, either:

1. Run as root (not recommended)
2. Use capabilities (Linux):
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
# Check logs
sudo journalctl -u muti-metroo -n 50

# Test config manually
sudo -u muti-metroo muti-metroo run -c /etc/muti-metroo/config.yaml
```

### Permission Denied

```bash
# Check ownership
ls -la /etc/muti-metroo/
ls -la /var/lib/muti-metroo/

# Fix permissions
sudo chown -R muti-metroo:muti-metroo /var/lib/muti-metroo
```

### Port Already in Use

```bash
sudo lsof -i :4433
sudo netstat -tlnp | grep 4433
```
