---
title: Installation
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-hammering.png" alt="Mole building" style={{maxWidth: '180px'}} />
</div>

# Installation

This guide covers all methods for installing Muti Metroo.

## Download Binary

The easiest way to install Muti Metroo is to download a pre-built binary for your platform.

**[Download Muti Metroo](/download)**

Pre-built binaries are available for:

- **macOS**: Apple Silicon (arm64) and Intel (amd64) - both installer packages and standalone binaries
- **Linux**: x86_64 (amd64) and ARM64 (aarch64)
- **Windows**: x86_64 (amd64) and ARM64

### Quick Install (Linux/macOS)

```bash
# Download for your platform (example: Linux amd64)
curl -L -o muti-metroo https://mutimetroo.com/downloads/latest/muti-metroo-linux-amd64

# Make executable and install
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Verify installation
muti-metroo --version
```

See the [Download page](/download) for platform-specific instructions.

## Docker Deployment

For containerized deployments, see [Docker Deployment](../deployment/docker) for Docker Compose setup and configuration examples.

## System Service Installation

Install Muti Metroo as a system service for automatic startup.

### Linux (systemd)

```bash
# Install as service (requires root)
sudo muti-metroo service install -c /etc/muti-metroo/config.yaml

# Enable and start
sudo systemctl enable muti-metroo
sudo systemctl start muti-metroo

# Check status
sudo systemctl status muti-metroo

# View logs
sudo journalctl -u muti-metroo -f
```

### Windows

```powershell
# Install as Windows Service (requires Administrator)
muti-metroo.exe service install -c C:\ProgramData\muti-metroo\config.yaml

# Start service
sc start muti-metroo

# Check status
sc query muti-metroo
```

### Uninstall Service

```bash
# Linux
sudo muti-metroo service uninstall

# Windows (as Administrator)
muti-metroo.exe service uninstall
```

## Directory Structure

After installation, you will typically have:

```
/etc/muti-metroo/          # Configuration (Linux)
  config.yaml              # Main configuration file
/var/lib/muti-metroo/      # Data directory (Linux)
  agent_id                 # Agent identity file
/var/log/muti-metroo/      # Logs (if using file logging)
```

For development:

```
./config.yaml              # Configuration file
./data/                    # Data directory
  agent_id                 # Agent identity (128-bit hex)
  keypair.json             # E2E encryption keypair
./certs/                   # TLS certificates
  ca.crt                   # Certificate Authority
  ca.key                   # CA private key
  <name>.crt               # Agent certificate (named after -n flag)
  <name>.key               # Agent private key
```

## Verify Installation

After installation, verify everything works:

```bash
# Initialize agent identity
muti-metroo init -d ./data

# Check the generated agent ID
cat ./data/agent_id

# Generate test certificates
muti-metroo cert ca -n "Test CA" -o ./certs
muti-metroo cert agent -n "test-agent" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs

# Verify certificates (output file uses common name)
muti-metroo cert info ./certs/test-agent.crt
```

## Next Steps

- [Quick Start](quick-start) - Create your first configuration
- [Interactive Setup](interactive-setup) - Use the setup wizard
- [Configuration Reference](../configuration/overview) - All configuration options
