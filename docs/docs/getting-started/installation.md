---
title: Installation
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-hammering.png" alt="Mole building" style={{maxWidth: '180px'}} />
</div>

# Installation

This guide covers all methods for installing Muti Metroo.

## Build from Source

### Prerequisites

- Go 1.23 or later ([download](https://go.dev/dl/))
- Git
- Make (optional)

### Clone and Build

```bash
# Clone the repository
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4

# Build using Make (recommended)
make build

# Or build directly with Go
go build -o build/muti-metroo ./cmd/muti-metroo
```

The compiled binary will be at `muti-metroo`.

### Verify Installation

```bash
muti-metroo --version
muti-metroo --help
```

### Install to PATH (Optional)

```bash
# Install to $GOPATH/bin
make install

# Or manually copy
sudo cp muti-metroo /usr/local/bin/
```

## Cross-Compilation

Build for different platforms:

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o build/muti-metroo-linux-amd64 ./cmd/muti-metroo

# Linux ARM64 (Raspberry Pi, AWS Graviton)
GOOS=linux GOARCH=arm64 go build -o build/muti-metroo-linux-arm64 ./cmd/muti-metroo

# Windows
GOOS=windows GOARCH=amd64 go build -o build/muti-metroo-windows-amd64.exe ./cmd/muti-metroo

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o build/muti-metroo-darwin-amd64 ./cmd/muti-metroo

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o build/muti-metroo-darwin-arm64 ./cmd/muti-metroo
```

### Static Binary

Build a fully static binary (no external dependencies):

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o build/muti-metroo ./cmd/muti-metroo
```

## Docker Installation

### Build Docker Image

```bash
# Build the image
docker build -t muti-metroo .

# Or use Docker Compose
docker compose build
```

### Run with Docker

```bash
docker run -d --name muti-metroo \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/app/certs \
  -p 1080:1080 \
  -p 4433:4433/udp \
  -p 8080:8080 \
  muti-metroo
```

### Docker Compose

See [Docker Deployment](../deployment/docker) for complete Docker Compose setup.

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
  control.sock             # Control socket
/var/log/muti-metroo/      # Logs (if using file logging)
```

For development:

```
./config.yaml              # Configuration file
./data/                    # Data directory
  agent_id                 # Agent identity
  control.sock             # Control socket
./certs/                   # TLS certificates
  ca.crt                   # Certificate Authority
  agent.crt                # Agent certificate
  agent.key                # Agent private key
```

## Verify Installation

After installation, verify everything works:

```bash
# Initialize agent identity
muti-metroo init -d ./data

# Check the generated agent ID
cat ./data/agent_id

# Generate test certificates
muti-metroo cert ca -n "Test CA"
muti-metroo cert agent -n "test-agent"

# Verify certificates
muti-metroo cert info ./certs/agent.crt
```

## Next Steps

- [Quick Start](quick-start) - Create your first configuration
- [Interactive Setup](interactive-setup) - Use the setup wizard
- [Configuration Reference](../configuration/overview) - All configuration options
