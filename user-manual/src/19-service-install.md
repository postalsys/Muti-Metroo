# Service Installation

Install Muti Metroo as a system service for automatic startup and management.

## Installation Methods

There are two ways to install as a service:

| Method | Command | Use Case |
|--------|---------|----------|
| **Config file** | `service install -c config.yaml` | Traditional deployment |
| **Embedded config** | Via setup wizard | Single-file deployment |

## Embedded Configuration Services

When using embedded configuration (via the setup wizard), the service installation:

1. Copies the binary to a standard location:
   - Linux/macOS: `/usr/local/bin/<service-name>`
   - Windows: `C:\Program Files\<service-name>\<service-name>.exe`

2. Creates service definition without `-c` flag or `run` command:
   ```bash
   # Embedded config service (no arguments needed)
   ExecStart=/usr/local/bin/my-agent
   ```

3. Uses the custom service name you specified

To update an embedded config service:

```bash
# Stop service
sudo systemctl stop my-agent

# Edit embedded config using regular binary
muti-metroo setup -c /usr/local/bin/my-agent

# Restart service
sudo systemctl start my-agent
```

## Linux (systemd)

### Installation

```bash
# Install as service (requires root) - auto-starts immediately
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml
```

This copies the binary to `/usr/local/bin/muti-metroo`, creates a systemd unit file at `/etc/systemd/system/muti-metroo.service`, enables boot startup, and starts the service immediately.

> **Binary Deployment:** By default, `service install` copies the binary to a standard system location (`/usr/local/bin` on Linux/macOS, `C:\Program Files\<name>` on Windows). Use `--deploy=false` to skip this and reference the binary at its current path.

### Service Management

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

> **Note:** The `service install` command automatically enables and starts the service. You don't need to run `systemctl enable` or `systemctl start` manually.

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

Windows Service installation requires Administrator privileges. If you don't have admin access, see the **Windows User Service** section below for a non-admin alternative using the Registry Run key.

### Installation

Run as Administrator (auto-starts immediately):

```powershell
muti-metroo.exe service install -c C:\ProgramData\muti-metroo\config.yaml
```

The service starts immediately after installation. No reboot required.

### Service Management

```powershell
# Status
sc query muti-metroo

# Restart (after config changes)
sc stop muti-metroo && sc start muti-metroo

# Stop
sc stop muti-metroo
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

## Windows User Service (Registry Run)

For users without Administrator access, install as a user service using the Registry Run key and the DLL.

### Requirements

- `muti-metroo.exe` - CLI for installation and management
- `muti-metroo.dll` - DLL for background execution via rundll32
- `config.yaml` - Configuration file

### Installation via CLI

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

### Installation via DLL Install Export

When the CLI executable is not available, the DLL can install itself as a user service directly:

```powershell
rundll32.exe C:\path\to\muti-metroo.dll,Install C:\path\to\config.yaml
```

The `Install` export performs the following:

1. If an existing user service is detected, stops it and uninstalls it (upgrade handling)
2. Creates the Registry Run key at `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
3. Writes a `service.info` file for status tracking and uninstall
4. Starts the agent immediately via a scheduled task

The config file and DLL must both exist at the specified paths before calling `Install`. The service name defaults to `muti-metroo` (Registry value: `MutiMetroo`).

This is functionally equivalent to running `muti-metroo service install --user --dll` via the CLI, and is useful for custom deployment tools and automated installers that only have the DLL.

### What Both Methods Create

Both installation methods create a Registry Run entry at `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` that:

- Starts immediately after installation (no reboot required)
- Runs automatically at each user logon
- Uses `rundll32.exe` to execute the DLL
- Runs with current user privileges
- No console window (background execution)

### Service Management

```powershell
# Check status
muti-metroo service status

# View registry entry
reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v MutiMetroo

# Stop manually (if needed)
taskkill /F /IM rundll32.exe
```

> **Note:** To restart the service, uninstall and reinstall it, or log out and log back in.

### Uninstall

```powershell
# Uninstall (no admin required)
muti-metroo service uninstall
```

### Comparison: Windows Service vs Registry Run

| Feature               | Windows Service        | Registry Run         |
|-----------------------|------------------------|----------------------|
| Requires admin        | Yes                    | No                   |
| Auto-restart on crash | Yes                    | No                   |
| Start timing          | At boot (before login) | At user logon        |
| Console window        | No                     | No                   |
| Runs as               | SYSTEM/service account | Current user         |
| Process name          | `muti-metroo.exe`      | `rundll32.exe`       |
| Requires DLL          | No                     | Yes                  |

**Choose Windows Service when:**

- You have Administrator access
- Agent must start before any user logs in
- You need automatic crash recovery

**Choose Registry Run when:**

- You don't have Administrator access
- User-level execution is acceptable
- Minimal installation footprint desired

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

## PM2 Process Manager

[PM2](https://pm2.keymetrics.io/) is a cross-platform process manager that can manage Muti Metroo alongside other applications.

### Quick Start

```bash
# Install PM2
npm install -g pm2

# Start Muti Metroo
pm2 start muti-metroo -- run -c /etc/muti-metroo/config.yaml

# Save and enable startup
pm2 save
pm2 startup
```

### Ecosystem File

Create `ecosystem.config.js` for reproducible deployments:

```javascript
// ecosystem.config.js
module.exports = {
  apps: [{
    name: 'muti-metroo',
    script: '/usr/local/bin/muti-metroo',
    args: 'run -c /etc/muti-metroo/config.yaml',
    cwd: '/etc/muti-metroo',
    autorestart: true,
    max_restarts: 10,
    restart_delay: 5000,
    error_file: '/var/log/muti-metroo/error.log',
    out_file: '/var/log/muti-metroo/out.log',
    merge_logs: true,
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z'
  }]
};
```

Start with ecosystem file:

```bash
pm2 start /etc/muti-metroo/ecosystem.config.js
pm2 save
```

### Multiple Agents

```javascript
// ecosystem.config.js
module.exports = {
  apps: [
    {
      name: 'muti-metroo-ingress',
      script: '/usr/local/bin/muti-metroo',
      args: 'run -c /etc/muti-metroo/ingress.yaml',
      autorestart: true
    },
    {
      name: 'muti-metroo-exit',
      script: '/usr/local/bin/muti-metroo',
      args: 'run -c /etc/muti-metroo/exit.yaml',
      autorestart: true
    }
  ]
};
```

### PM2 Commands

```bash
# Status
pm2 status
pm2 show muti-metroo

# Logs
pm2 logs muti-metroo

# Restart/Stop
pm2 restart muti-metroo
pm2 stop muti-metroo

# Monitoring
pm2 monit
```

### Log Rotation

```bash
pm2 install pm2-logrotate
pm2 set pm2-logrotate:max_size 10M
pm2 set pm2-logrotate:retain 7
```

### Windows

```powershell
npm install -g pm2
pm2 start C:\ProgramData\muti-metroo\muti-metroo.exe -- run -c C:\ProgramData\muti-metroo\config.yaml
pm2 save
pm2-startup install
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
