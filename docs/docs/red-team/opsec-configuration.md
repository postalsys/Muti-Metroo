---
title: OPSEC Configuration
sidebar_label: OPSEC Configuration
sidebar_position: 2
---

# OPSEC Configuration

This page covers operational security configuration options for minimizing detection signatures.

## Protocol Identifier Customization

By default, Muti Metroo uses identifiable protocol strings. Disable all custom identifiers for stealth:

```yaml
protocol:
  alpn: ""           # Disable custom ALPN (QUIC/TLS)
  http_header: ""    # Disable X-Muti-Metroo-Protocol header
  ws_subprotocol: "" # Disable WebSocket subprotocol
```

| Identifier | Default Value | Network Visibility |
|------------|---------------|-------------------|
| ALPN | `muti-metroo/1` | TLS ClientHello, visible to middleboxes |
| HTTP Header | `X-Muti-Metroo-Protocol` | HTTP/2 headers |
| WS Subprotocol | `muti-metroo/1` | WebSocket upgrade request |

### ALPN Impersonation

Instead of disabling ALPN entirely, you can set it to mimic legitimate applications. This may help blend traffic with expected protocols on the network:

```yaml
protocol:
  alpn: "h2"              # Standard HTTP/2 (nginx, Apache, most web servers)
  # alpn: "http/1.1"      # HTTP/1.1
  # alpn: "grpc"          # gRPC services
  # alpn: "dot"           # DNS over TLS
  # alpn: "imap"          # IMAP over TLS
  # alpn: "xmpp-client"   # XMPP/Jabber
```

**Common ALPN strings by application:**

| ALPN Value | Typical Application |
|------------|---------------------|
| `h2` | Nginx, Apache, Caddy, most HTTPS servers |
| `http/1.1` | Legacy HTTP servers |
| `grpc` | gRPC microservices |
| `dot` | DNS over TLS (port 853) |
| `spdy/3.1` | Legacy SPDY (rare) |
| `stun.turn` | WebRTC TURN servers |
| `webrtc` | WebRTC data channels |
| `imap` | IMAP mail servers |
| `pop3` | POP3 mail servers |
| `xmpp-client` | Jabber/XMPP chat |

Choose an ALPN value that matches services expected on your target network or cover infrastructure.

## HTTP Endpoint Hardening

The HTTP API can leak operational information. Minimize exposure:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"  # Localhost only
  minimal: true              # Only /health, /healthz, /ready
```

Or with granular control:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  pprof: false       # NEVER enable in operations
  dashboard: false   # Exposes topology
  remote_api: false  # Exposes agent list
```

Disabled endpoints return HTTP 404 (indistinguishable from non-existent paths).

## Environment Variable Substitution

Configs support environment variables for credential separation:

```yaml
socks5:
  auth:
    users:
      - username: "${SOCKS_USER}"
        password: "${SOCKS_PASS}"

shell:
  password_hash: "${SHELL_HASH}"

management:
  public_key: "${MGMT_PUBKEY}"
```

This allows credentials to be passed at runtime without filesystem artifacts.
