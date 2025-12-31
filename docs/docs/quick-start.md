---
title: Quick Start
sidebar_position: 4
---

# Quick Start

This guide walks you through manually setting up a Muti Metroo agent.

## Initialize Agent Identity

First, create an agent identity:

```bash
./build/muti-metroo init -d ./data
```

This generates a unique 128-bit Agent ID and stores it in the data directory.

## Generate TLS Certificates

Generate a Certificate Authority and agent certificates:

```bash
# Create CA
./build/muti-metroo cert ca -n "My Mesh CA" -o ./certs

# Generate agent certificate
./build/muti-metroo cert agent -n "agent-1" --dns "agent1.example.com" --ip "192.168.1.10"
```

## Create Configuration File

Create config.yaml with basic settings:

```yaml
agent:
  id: "auto"
  data_dir: "./data"
  log_level: "info"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: ":8080"
```

## Run the Agent

```bash
./build/muti-metroo run -c ./config.yaml
```

## Test the Setup

```bash
# Check health
curl http://localhost:8080/health

# View metrics
curl http://localhost:8080/metrics

# Test SOCKS5 proxy
curl -x socks5://localhost:1080 https://example.com
```

## Next Steps

- [Configuration](configuration) - Full configuration reference
- [Architecture](architecture/overview) - Understand agent roles
