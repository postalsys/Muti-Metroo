---
title: Configuration
sidebar_position: 6
---

# Configuration Reference

Complete configuration file reference for Muti Metroo.

## Configuration File Format

Muti Metroo uses YAML configuration files with support for environment variable substitution.

### Environment Variables

Use `${VAR}` or `${VAR:-default}` syntax:

```yaml
agent:
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"
```

## Configuration Sections

### Agent Identity

```yaml
agent:
  id: "auto"                 # Auto-generate or specify hex string
  display_name: "My Agent"   # Human-readable name (Unicode allowed)
  data_dir: "./data"         # Persistent state directory
  log_level: "info"          # debug, info, warn, error
  log_format: "text"         # text, json
```

### Transport Listeners

```yaml
listeners:
  - transport: quic          # quic, h2, or ws
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"  # Optional: require client certs
      
      # Inline PEM (takes precedence over file paths)
      cert_pem: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
      key_pem: |
        -----BEGIN PRIVATE KEY-----
        ...
        -----END PRIVATE KEY-----
```

### Peer Connections

```yaml
peers:
  - id: "abc123..."           # Expected peer AgentID
    transport: quic
    address: "192.168.1.50:4433"
    tls:
      ca: "./certs/peer-ca.crt"
```

### SOCKS5 Server

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$..."  # bcrypt hash
  max_connections: 1000
```

### Exit Node

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "0.0.0.0/0"      # Default route
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s
```

### Routing

```yaml
routing:
  advertise_interval: 2m      # Route advertisement frequency
  node_info_interval: 2m      # Node info advertisement frequency
  route_ttl: 5m               # Route expiration time
  max_hops: 16                # Maximum path length
```

### Connection Tuning

```yaml
connections:
  idle_threshold: 30s         # Keepalive interval
  timeout: 90s                # Connection timeout
  reconnect:
    initial_delay: 1s
    max_delay: 60s
    multiplier: 2.0
    jitter: 0.2
    max_retries: 0            # 0 = infinite
```

### Resource Limits

```yaml
limits:
  max_streams_per_peer: 1000
  max_streams_total: 10000
  max_pending_opens: 100
  stream_open_timeout: 30s
  buffer_size: 262144         # 256 KB
```

### HTTP API Server

```yaml
http:
  enabled: true
  address: ":8080"
  read_timeout: 10s
  write_timeout: 10s
```

### Control Socket

```yaml
control:
  enabled: true
  socket_path: "./data/control.sock"
```

### RPC (Remote Procedure Call)

```yaml
rpc:
  enabled: false              # Disabled by default
  whitelist:                  # Allowed commands
    - whoami
    - hostname
    - ip
  password_hash: ""           # bcrypt hash
  timeout: 60s
```

### File Transfer

```yaml
file_transfer:
  enabled: false
  max_file_size: 0            # 0 = unlimited
  allowed_paths:              # Allowed path prefixes
    - /tmp
    - /home/user/uploads
  password_hash: ""           # bcrypt hash
```

## Complete Example

See `configs/example.yaml` in the repository for a fully commented configuration file.
