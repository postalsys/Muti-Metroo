# Configuration Reference

This chapter provides a complete reference for all Muti Metroo configuration options.

## Configuration File

Muti Metroo uses YAML configuration files. Specify the configuration path with:

```bash
muti-metroo run -c /path/to/config.yaml
```

## Configuration Structure

```yaml
# Agent identity and logging
agent:
  id: "auto"                    # Agent ID (auto-generate or hex string)
  display_name: "My Agent"      # Human-readable name
  data_dir: "./data"            # Persistent state directory
  log_level: "info"             # debug, info, warn, error
  log_format: "text"            # text, json

# Global TLS configuration
tls:
  ca: "./certs/ca.crt"          # CA certificate
  cert: "./certs/agent.crt"     # Agent certificate
  key: "./certs/agent.key"      # Private key
  mtls: false                   # Enable mutual TLS

# Protocol identifiers (OPSEC)
protocol:
  alpn: "muti-metroo/1"         # ALPN for QUIC/TLS
  http_header: "X-Muti-Metroo-Protocol"
  ws_subprotocol: "muti-metroo/1"

# Transport listeners
listeners:
  - transport: quic             # quic, h2, ws
    address: "0.0.0.0:4433"

# Peer connections
peers:
  - id: "abc123..."
    transport: quic
    address: "192.168.1.10:4433"

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
  node_info_interval: 2m
  route_ttl: 5m
  max_hops: 16

# Connection tuning
connections:
  idle_threshold: 5m
  timeout: 90s
  keepalive_jitter: 0.2
  reconnect:
    initial_delay: 1s
    max_delay: 60s
    multiplier: 2.0
    jitter: 0.2
    max_retries: 0

# Resource limits
limits:
  max_streams_per_peer: 1000
  max_streams_total: 10000
  max_pending_opens: 100
  stream_open_timeout: 30s
  buffer_size: 262144

# HTTP API
http:
  enabled: true
  address: ":8080"
  minimal: false
  pprof: false
  dashboard: true
  remote_api: true

# Remote shell
shell:
  enabled: false
  whitelist: []
  password_hash: ""
  timeout: 0s
  max_sessions: 0

# File transfer
file_transfer:
  enabled: false
  max_file_size: 524288000
  allowed_paths: []
  password_hash: ""

# Management key encryption
management:
  public_key: ""
  private_key: ""

# UDP relay
udp:
  enabled: false
  max_associations: 1000
  idle_timeout: 5m
  max_datagram_size: 1472

# Port forwarding
forward:
  endpoints: []
  listeners: []
```

## Agent Section

```yaml
agent:
  id: "auto"                    # "auto" or 32-char hex string
  display_name: "My Agent"      # Shown in dashboard
  data_dir: "./data"            # Where to store state
  log_level: "info"             # debug, info, warn, error
  log_format: "text"            # text or json
```

## Listeners Section

Configure transport listeners:

```yaml
listeners:
  # QUIC listener
  - transport: quic
    address: "0.0.0.0:4433"

  # HTTP/2 listener
  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"

  # WebSocket listener
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"

  # Plaintext (behind reverse proxy)
  - transport: ws
    address: "127.0.0.1:8080"
    path: "/mesh"
    plaintext: true
```

## Peers Section

Configure outbound peer connections:

```yaml
peers:
  # QUIC peer
  - id: "abc123def456..."
    transport: quic
    address: "192.168.1.10:4433"

  # HTTP/2 peer
  - id: "def456abc123..."
    transport: h2
    address: "relay.example.com:8443"
    path: "/mesh"

  # WebSocket with proxy
  - id: "789xyz..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    proxy: "http://proxy.corp.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
```

**Connection direction is arbitrary**: An agent with `peers` configured acts as a dialer (client), while the target agent must have `listeners`. However, once connected, **both agents can initiate virtual streams in either direction**. The connection direction does not affect which agent can be ingress, transit, or exit - choose based on network constraints (firewalls, NAT), not functionality. See the Agent Roles chapter for details.

## SOCKS5 Section

Configure the SOCKS5 proxy ingress:

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$..."
  max_connections: 1000
```

Generate password hashes with:

```bash
muti-metroo hash --cost 12
```

## Exit Section

Configure exit node routing:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "0.0.0.0/0"
  domain_routes:
    - "api.internal.corp"
    - "*.example.com"
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

## HTTP API Section

Configure the HTTP API server:

```yaml
http:
  enabled: true
  address: ":8080"
  read_timeout: 10s
  write_timeout: 10s
  minimal: false               # Only health endpoints
  pprof: false                 # Debug endpoints
  dashboard: true              # Web dashboard
  remote_api: true             # Remote agent APIs
```

## Environment Variables

All configuration values support environment variable substitution:

```yaml
agent:
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"

socks5:
  auth:
    users:
      - username: "${SOCKS_USER}"
        password_hash: "${SOCKS_PASS_HASH}"
```

Syntax:
- `${VAR}` - Use environment variable (error if not set)
- `${VAR:-default}` - Use default if variable not set

## Minimal Configuration

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
