---
title: Example Configurations
sidebar_label: Example Configurations
sidebar_position: 6
---

# Example Configurations

Ready-to-use configuration templates for common operational scenarios.

## Minimal Transit Node

Relay-only node with minimum footprint:

```yaml
agent:
  data_dir: "/var/lib/app-cache"
  log_level: "error"

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

## Full C2 Endpoint

Complete capability for target access:

```yaml
agent:
  data_dir: "/opt/.cache/app"
  log_level: "error"

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

Entry point for operator traffic:

```yaml
protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

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
