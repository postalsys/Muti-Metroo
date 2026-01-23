# Privacy & Security Configuration

This chapter covers configuration options for enhancing privacy, reducing network fingerprint, and securing your deployment.

## Protocol Identifier Customization

By default, Muti Metroo uses identifiable protocol strings. You can customize or disable these identifiers for compatibility with corporate policies or firewall rules:

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

### ALPN Compatibility

Set ALPN to match standard protocols for firewall compatibility:

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

Choose an ALPN value that matches your network environment.

## TLS Fingerprint Customization

TLS fingerprinting (JA3/JA4) identifies clients by hashing their TLS ClientHello parameters - cipher suites, extensions, and curves. Network monitoring tools use these fingerprints to detect unusual TLS clients.

Muti Metroo can mimic popular browser fingerprints for outbound connections:

```yaml
tls:
  fingerprint:
    preset: "chrome"  # Mimic Chrome browser
```

### Available Presets

| Preset | Description |
|--------|-------------|
| `disabled` | Go standard library (default) |
| `chrome` | Latest Chrome browser |
| `firefox` | Latest Firefox browser |
| `safari` | Safari browser |
| `edge` | Microsoft Edge |
| `ios` | iOS Safari |
| `android` | Android Chrome |
| `random` | Randomized per connection |

### Transport Support

| Transport | Fingerprint Support |
|-----------|---------------------|
| HTTP/2 | Full customization |
| WebSocket | Full customization |
| QUIC | Not supported (QUIC uses internal TLS 1.3) |

### Usage Notes

- Fingerprint customization only affects **outbound** peer connections
- Listeners (servers) are not affected - servers don't send ClientHello
- Combine with protocol identifier customization for maximum effectiveness:

```yaml
tls:
  fingerprint:
    preset: "chrome"

protocol:
  alpn: ""           # Disable custom ALPN
  http_header: ""    # Disable custom header
  ws_subprotocol: "" # Disable custom subprotocol

listeners:
  - transport: h2
    address: ":443"  # Standard HTTPS port
```

## HTTP Endpoint Security

The HTTP API exposes operational information. Minimize exposure based on your security requirements:

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
  pprof: false       # Disable debug endpoints in production
  dashboard: false   # Disable topology visualization
  remote_api: false  # Disable agent list access
```

Disabled endpoints return HTTP 404 (indistinguishable from non-existent paths).

## Environment Variable Substitution

Use environment variables for secrets management - avoids storing credentials in configuration files:

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

### Minimal Logging

For production deployments where log storage is limited:

```bash
muti-metroo run -c config.yaml 2>/dev/null
```

## Network Configuration

### Connection Timing

Add jitter to keepalive to distribute network load:

```yaml
connections:
  keepalive_jitter: 0.3    # 30% random jitter
```

### Transport Selection

| Scenario | Recommended Transport | Reason |
|----------|----------------------|--------|
| Standard deployment | QUIC on 443 | Modern, efficient |
| Corporate environment | HTTP/2 on 443 | Compatible with proxies |
| Legacy firewall | WebSocket on 443 | Widely supported |

## Example Configurations

### Minimal Configuration

Reduced footprint for production deployments:

```yaml
agent:
  id: "auto"
  data_dir: "/var/lib/muti-metroo"
  log_level: "warn"
  log_format: "json"

tls:
  fingerprint:
    preset: "chrome"

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

### Full Management Configuration

Full visibility for administration:

```yaml
agent:
  id: "auto"
  display_name: "Management Console"
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

## Signing Key Security

If using sleep mode in untrusted environments, protect signing keys with the same care as certificate private keys.

### Key Distribution Model

```
Operator Station      Remote Agents
(has private key)     (public key only)
      |                    |
      | Signed commands    |
      |------------------->|
      |                    | (verify only)
```

### Best Practices

1. **Generate keys offline** on a secure machine
2. **Never store signing private keys** on remote agents
3. **Limit operator access** to authorized personnel only
4. **Rotate keys periodically** for long-running operations
5. **Destroy keys** after operation concludes

### Environment Variables

Pass signing keys via environment for additional security:

```yaml
management:
  signing_public_key: "${SIGNING_PUBKEY}"
  signing_private_key: "${SIGNING_PRIVKEY}"
```

```bash
export SIGNING_PUBKEY="a1b2c3d4..."
export SIGNING_PRIVKEY="e5f6a7b8..."
muti-metroo run -c config.yaml
```

## Deployment Checklist

Before deployment, verify:

- [ ] Protocol identifiers configured appropriately
- [ ] TLS fingerprint preset configured (if using HTTP/2 or WebSocket)
- [ ] HTTP API bound to localhost or disabled
- [ ] Dashboard disabled on remote agents
- [ ] pprof disabled in production
- [ ] Credentials passed via environment variables
- [ ] Keepalive jitter configured
- [ ] Log level set appropriately
- [ ] Management keys configured (if using)
- [ ] Signing keys configured (if using sleep mode in untrusted environments)
