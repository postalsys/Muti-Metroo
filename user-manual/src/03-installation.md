# Installation

This chapter covers all methods for installing Muti Metroo.

## Download Binary

Pre-built binaries are available for all major platforms:

| Platform | Architecture | Binary Name |
|----------|--------------|-------------|
| Linux | x86_64 | `muti-metroo-linux-amd64` |
| Linux | ARM64 | `muti-metroo-linux-arm64` |
| macOS | Apple Silicon | `muti-metroo-darwin-arm64` |
| macOS | Intel | `muti-metroo-darwin-amd64` |
| Windows | x86_64 | `muti-metroo-windows-amd64.exe` |
| Windows | ARM64 | `muti-metroo-windows-arm64.exe` |
| Windows | x86_64 (DLL) | `muti-metroo-windows-amd64.dll` |

Download from: `https://github.com/postalsys/Muti-Metroo/releases`

### Linux/macOS Installation

```bash
# Download (example: Linux amd64)
curl -L -o muti-metroo \
  https://download.mutimetroo.com/linux-amd64/muti-metroo

# Make executable and install
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Verify
muti-metroo --version
```

### Windows Installation

1. Download the appropriate `.exe` file from the releases page
2. Place in a directory (e.g., `C:\muti-metroo\`)
3. Optionally add to PATH

```powershell
# Verify installation
.\muti-metroo.exe --version
```

### DLL Mode (Windows)

For background execution without a console window, use the DLL variant with `rundll32.exe`:

```powershell
rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml
```

**Note:** Embedded configuration is not supported for DLLs (incompatible with UPX compression). Use the `.exe` for single-file deployment with embedded config.

On ARM64 Windows, use the x64 emulation layer's rundll32:

```powershell
C:\Windows\SysWOW64\rundll32.exe C:\path\to\muti-metroo.dll,Run C:\path\to\config.yaml
```

**Process Behavior:**

The DLL runs as a background process (not a Windows service):

- No console window - runs silently in the background
- Appears as `rundll32.exe` in Task Manager
- Does NOT survive reboots - use Registry Run for persistence
- Cannot be managed via Services console (`services.msc`)

To terminate: `taskkill /F /IM rundll32.exe`

**Use .exe for:**

- Normal operation with console output
- Persistent service installation (`muti-metroo service install`)
- Interactive commands (shell, upload, download)

**Use .dll for:**

- Background execution without console window
- Quick deployments without service installation
- Scenarios where hiding the console is important

**Persistence with Registry Run (Recommended):**

Use the CLI to install as a user service (no admin required):

```powershell
muti-metroo service install --user --dll C:\path\to\muti-metroo.dll -c C:\path\to\config.yaml
```

This creates a Registry Run entry at `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` that starts automatically at user logon.

**Manual Registry Setup:**

```powershell
# Add Registry Run entry (no admin required)
$dll = "C:\Users\$env:USERNAME\muti-metroo\muti-metroo.dll"
$cfg = "C:\Users\$env:USERNAME\muti-metroo\config.yaml"
$cmd = "rundll32.exe `"$dll`",Run `"$cfg`""

Set-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" `
    -Name "MutiMetroo" -Value $cmd
```

For system-wide startup (requires admin), use Windows Service instead:

```powershell
muti-metroo.exe service install -c C:\ProgramData\muti-metroo\config.yaml
```

## Docker Deployment

For containerized deployments:

```bash
# Build from source
docker build -t muti-metroo .

# Run with config
docker run -v $(pwd)/config.yaml:/etc/muti-metroo/config.yaml \
  -p 4433:4433/udp \
  -p 1080:1080 \
  -p 8080:8080 \
  muti-metroo run -c /etc/muti-metroo/config.yaml
```

## Directory Structure

After installation, you will typically have:

### Production Layout (Linux)

```
/usr/local/bin/muti-metroo     # Binary
/etc/muti-metroo/              # Configuration
  config.yaml                  # Main configuration file
  certs/                       # TLS certificates
    ca.crt
    agent.crt
    agent.key
/var/lib/muti-metroo/          # Data directory
  agent_id                     # Agent identity
  keypair.json                 # E2E encryption keypair
```

### Development Layout

```
./muti-metroo                  # Binary
./config.yaml                  # Configuration file
./data/                        # Data directory
  agent_id                     # Agent identity (128-bit hex)
  keypair.json                 # E2E encryption keypair
./certs/                       # TLS certificates
  ca.crt                       # Certificate Authority
  ca.key                       # CA private key
  agent.crt                    # Agent certificate
  agent.key                    # Agent private key
```

### Windows Layout

```
C:\Program Files\muti-metroo\
  muti-metroo.exe              # Binary
C:\ProgramData\muti-metroo\
  config.yaml                  # Configuration
  data\                        # Data directory
    agent_id
  certs\                       # Certificates
    ca.crt
    agent.crt
    agent.key
```

## Verify Installation

After installation, verify everything works:

```bash
# Initialize agent identity
muti-metroo init -d ./data

# Check the generated agent ID
cat ./data/agent_id

# Generate test certificates
muti-metroo cert ca --cn "Test CA" -o ./certs
muti-metroo cert agent --cn "test-agent" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs

# Verify certificate
muti-metroo cert info ./certs/test-agent.crt
```

## System Requirements

- **OS**: Linux, macOS, or Windows
- **Architecture**: x86_64 or ARM64
- **Memory**: Minimum 64MB, recommended 256MB+
- **Network**: UDP port for QUIC, TCP ports for HTTP/2 and WebSocket
- **Privileges**: No root required (runs in userspace)

## Firewall Configuration

Ensure the following ports are accessible:

| Port | Protocol | Purpose |
|------|----------|---------|
| 4433 | UDP | QUIC transport (default) |
| 8443 | TCP | HTTP/2 transport (optional) |
| 443 | TCP | WebSocket transport (optional) |
| 1080 | TCP | SOCKS5 proxy (ingress) |
| 8080 | TCP | HTTP API and dashboard |

Example firewall rules:

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
