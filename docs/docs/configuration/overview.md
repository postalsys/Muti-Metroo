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

tls:
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

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

# Global TLS configuration (used by listeners and peers)
tls:
  ca: "./certs/ca.crt"          # CA certificate
  cert: "./certs/agent.crt"     # Agent certificate
  key: "./certs/agent.key"      # Private key
  mtls: false                   # Enable mutual TLS

# Protocol identifiers (OPSEC customization)
protocol:
  alpn: "muti-metroo/1"         # ALPN for QUIC/TLS (empty to disable)
  http_header: "X-Muti-Metroo-Protocol"  # HTTP header (empty to disable)
  ws_subprotocol: "muti-metroo/1"        # WebSocket subprotocol (empty to disable)

# Transport listeners
listeners:
  - transport: quic             # quic, h2, ws
    address: "0.0.0.0:4433"
    # Uses global TLS settings; per-listener overrides available via tls: section

# Peer connections
peers:
  - id: "abc123..."
    transport: quic
    address: "192.168.1.10:4433"
    # Uses global TLS settings; per-peer overrides available via tls: section

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
  node_info_interval: 2m   # Node info advertisement interval
  route_ttl: 5m
  max_hops: 16

# Connection tuning
connections:
  idle_threshold: 5m
  timeout: 90s
  reconnect:
    initial_delay: 1s
    max_delay: 60s
    multiplier: 2.0        # Exponential backoff multiplier
    jitter: 0.2            # Random jitter factor (0.0-1.0)
    max_retries: 0         # 0 = infinite retries

# Resource limits
limits:
  max_streams_per_peer: 1000
  max_streams_total: 10000
  max_pending_opens: 100     # Pending stream open requests
  stream_open_timeout: 30s   # Timeout for stream open
  buffer_size: 262144

# HTTP API
http:
  enabled: true
  address: ":8080"
  read_timeout: 10s
  write_timeout: 10s
  minimal: false               # When true, only health endpoints enabled
  metrics: true                # /metrics endpoint
  pprof: false                 # /debug/pprof/* endpoints
  dashboard: true              # /ui/*, /api/* endpoints
  remote_api: true             # /agents/*, /metrics/{id} endpoints

# Shell (remote commands)
shell:
  enabled: false
  whitelist: []
  password_hash: ""
  timeout: 0s                # Default command timeout (0 = no timeout)
  max_sessions: 10           # Concurrent session limit

# File transfer
file_transfer:
  enabled: false
  max_file_size: 524288000   # 500 MB default, 0 = unlimited
  allowed_paths: []
  password_hash: ""

# Management key encryption (for red team ops)
management:
  public_key: ""             # 64-char hex, add to ALL agents
  private_key: ""            # 64-char hex, ONLY on operators
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

## Next Steps

- [Agent Configuration](agent) - Identity and logging
- [Listeners](listeners) - Transport setup
- [Peers](peers) - Connecting to other agents
- [TLS Certificates](tls-certificates) - Certificate management
