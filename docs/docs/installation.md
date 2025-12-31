---
title: Installation
sidebar_position: 3
---

# Installation

This guide covers installing Muti Metroo from source or using Docker.

## Build from Source

### Prerequisites

- Go 1.23 or later
- Git
- Make (optional)

### Clone and Build

```bash
# Clone the repository
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4

# Build the binary
make build

# Or build manually with Go
go build -o build/muti-metroo ./cmd/muti-metroo
```

The compiled binary will be available at `./build/muti-metroo`.

### Verify Installation

```bash
./build/muti-metroo --version
```

## Docker Installation

Muti Metroo includes Docker support for development and production deployments.

### Build Docker Image

```bash
# Build the image
docker build -t muti-metroo .

# Or use Docker Compose
docker compose build
```

### Run with Docker

```bash
# Run with config file
docker run -v $(pwd)/config.yaml:/app/config.yaml \
           -v $(pwd)/data:/app/data \
           -v $(pwd)/certs:/app/certs \
           -p 1080:1080 -p 4433:4433/udp \
           muti-metroo
```

### Docker Compose Example

```yaml
services:
  agent:
    build: .
    ports:
      - "1080:1080"
      - "4433:4433/udp"
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
      - ./certs:/app/certs
    restart: unless-stopped
```

## Service Installation

Muti Metroo can be installed as a system service on Linux (systemd) and Windows.

### Linux (systemd)

```bash
# Install as service (requires root)
sudo ./build/muti-metroo service install -c /path/to/config.yaml

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
muti-metroo service install -c C:\path\to\config.yaml

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
muti-metroo service uninstall
```

## Next Steps

- [Quick Start](quick-start) - Manual configuration
- [Interactive Setup](interactive-setup) - Guided setup wizard
- [Configuration](configuration) - Configuration reference
