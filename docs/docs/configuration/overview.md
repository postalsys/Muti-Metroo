---
title: Configuration Overview
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole inspecting configuration" style={{maxWidth: '180px'}} />
</div>

# Configuration Reference

This section provides a complete reference for all Muti Metroo configuration options.

## Configuration File

Muti Metroo uses YAML configuration files. The default location is `./config.yaml`, but you can specify a different path:

```bash
muti-metroo run -c /path/to/config.yaml
```

## Configuration Sections

| Section | Purpose |
|---------|---------|
| [agent](agent) | Agent identity and logging |
| [listeners](listeners) | Transport listeners (QUIC, HTTP/2, WebSocket) |
| [peers](peers) | Outbound peer connections |
| [socks5](socks5) | SOCKS5 proxy configuration |
| [exit](exit) | Exit node routes and DNS |
| [tls-certificates](tls-certificates) | TLS/mTLS configuration |
| [environment-variables](environment-variables) | Environment variable substitution |

## Quick Reference

### Minimal Configuration

The simplest working configuration:

```yaml
agent:
  id: "auto"
  data_dir: "./data"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

exit:
  enabled: true
  routes:
    - "0.0.0.0/0"

http:
  enabled: true
  address: ":8080"
```

### Full Configuration

See [configs/example.yaml](https://git.aiateibad.ee/andris/Muti-Metroo-v4/src/branch/master/configs/example.yaml) for a fully commented configuration file.

## Configuration Structure

```yaml
# Agent identity and logging
agent:
  id: "auto"                    # Agent ID (auto-generate or hex string)
  display_name: "My Agent"      # Human-readable name
  data_dir: "./data"            # Persistent state directory
  log_level: "info"             # debug, info, warn, error
  log_format: "text"            # text, json

# Transport listeners
listeners:
  - transport: quic             # quic, h2, ws
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

# Peer connections
peers:
  - id: "abc123..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"

# SOCKS5 proxy
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
  max_connections: 1000

# Exit node
exit:
  enabled: false
  routes:
    - "10.0.0.0/8"
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s

# Routing settings
routing:
  advertise_interval: 2m
  route_ttl: 5m
  max_hops: 16

# Connection tuning
connections:
  idle_threshold: 30s
  timeout: 90s
  reconnect:
    initial_delay: 1s
    max_delay: 60s

# Resource limits
limits:
  max_streams_per_peer: 1000
  max_streams_total: 10000
  buffer_size: 262144

# HTTP API
http:
  enabled: true
  address: ":8080"

# Control socket
control:
  enabled: true
  socket_path: "./data/control.sock"

# RPC (remote commands)
rpc:
  enabled: false
  whitelist: []
  password_hash: ""

# File transfer
file_transfer:
  enabled: false
  allowed_paths: []
  password_hash: ""
```

## Environment Variables

All configuration values support environment variable substitution:

```yaml
agent:
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"

socks5:
  address: "${SOCKS_ADDR:-127.0.0.1:1080}"
  auth:
    users:
      - username: "${SOCKS_USER}"
        password_hash: "${SOCKS_PASS_HASH}"
```

Syntax:
- `${VAR}` - Use environment variable (error if not set)
- `${VAR:-default}` - Use default if variable not set

## Validation

Configuration is validated on startup. Common errors:

```
ERROR  Invalid configuration: socks5.address: invalid address format
ERROR  Invalid configuration: listeners[0].tls.cert: file not found
ERROR  Invalid configuration: peers[0].id: invalid agent ID format
```

## Reloading

Currently, configuration cannot be reloaded without restart. To apply changes:

```bash
# Stop agent (Ctrl+C or SIGTERM)
# Edit config.yaml
# Start agent
muti-metroo run -c ./config.yaml
```

## Configuration Best Practices

1. **Use environment variables** for secrets (passwords, keys)
2. **Keep configs in version control** (without secrets)
3. **Use display_name** for easier dashboard identification
4. **Start minimal** and add features as needed
5. **Test configuration** before production deployment

```bash
# Validate config without starting
muti-metroo validate -c ./config.yaml
```

## Next Steps

- [Agent Configuration](agent) - Identity and logging
- [Listeners](listeners) - Transport setup
- [Peers](peers) - Connecting to other agents
- [TLS Certificates](tls-certificates) - Certificate management
