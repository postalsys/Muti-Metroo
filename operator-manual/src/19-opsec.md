# OPSEC Configuration

This chapter covers operational security configuration options for minimizing detection signatures during red team operations.

## Protocol Identifier Customization

By default, Muti Metroo uses identifiable protocol strings. Disable or customize all identifiers for stealth:

```yaml
protocol:
  alpn: ""           # Disable custom ALPN
  http_header: ""    # Disable X-Muti-Metroo-Protocol header
  ws_subprotocol: "" # Disable WebSocket subprotocol
```

### Default Identifiers

| Identifier | Default Value | Network Visibility |
|------------|---------------|-------------------|
| ALPN | `muti-metroo/1` | TLS ClientHello, visible to middleboxes |
| HTTP Header | `X-Muti-Metroo-Protocol` | HTTP/2 headers |
| WS Subprotocol | `muti-metroo/1` | WebSocket upgrade request |

### ALPN Impersonation

Instead of disabling ALPN, set it to mimic legitimate applications:

```yaml
protocol:
  alpn: "h2"              # Standard HTTP/2
  # alpn: "http/1.1"      # HTTP/1.1
  # alpn: "grpc"          # gRPC services
```

**Common ALPN strings:**

| ALPN Value | Typical Application |
|------------|---------------------|
| `h2` | Nginx, Apache, Caddy, most HTTPS servers |
| `http/1.1` | Legacy HTTP servers |
| `grpc` | gRPC microservices |
| `dot` | DNS over TLS (port 853) |

Choose an ALPN value that matches services expected on your target network.

## HTTP Endpoint Hardening

The HTTP API can leak operational information. Minimize exposure:

### Minimal Mode

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"  # Localhost only
  minimal: true              # Only /health, /healthz, /ready
```

### Granular Control

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

Use environment variables for credential separation - no filesystem artifacts:

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

Pass credentials at runtime:

```bash
SOCKS_USER=operator SOCKS_PASS=secret muti-metroo run -c config.yaml
```

## Logging Configuration

### Reduce Log Verbosity

```yaml
agent:
  log_level: "warn"    # Only warnings and errors
  log_format: "json"   # Structured for parsing
```

### Log to Null

For maximum stealth (not recommended for debugging):

```bash
muti-metroo run -c config.yaml 2>/dev/null
```

## Network Fingerprint Reduction

### Connection Timing

Add jitter to keepalive to avoid beacon pattern detection:

```yaml
connections:
  keepalive_jitter: 0.3    # 30% random jitter
```

### Transport Selection

| Scenario | Recommended Transport | Reason |
|----------|----------------------|--------|
| Blending with web traffic | WebSocket on 443 | Standard HTTPS port |
| Corporate environment | HTTP/2 on 443 | Normal HTTPS traffic |
| Quick data center setup | QUIC on 443 | Looks like HTTPS/3 |

## Example OPSEC Configuration

### Field Agent (Maximum Stealth)

```yaml
agent:
  id: "auto"
  data_dir: "/var/lib/muti-metroo"
  log_level: "warn"
  log_format: "json"

protocol:
  alpn: "h2"
  http_header: ""
  ws_subprotocol: ""

listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/v1"

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

management:
  public_key: "${MGMT_PUBKEY}"
```

### Operator Station (Full Visibility)

```yaml
agent:
  id: "auto"
  display_name: "Operator Console"
  data_dir: "./data"
  log_level: "info"

protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

http:
  enabled: true
  address: "127.0.0.1:8080"
  dashboard: true
  remote_api: true

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```

## Checklist

Before deployment, verify:

- [ ] Protocol identifiers customized or disabled
- [ ] HTTP API bound to localhost or disabled
- [ ] Dashboard disabled on field agents
- [ ] pprof disabled everywhere
- [ ] Credentials passed via environment variables
- [ ] Keepalive jitter configured
- [ ] Log level set appropriately
- [ ] Management keys configured (if using)
