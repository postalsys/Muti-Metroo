---
title: Listeners
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring listeners" style={{maxWidth: '180px'}} />
</div>

# Listener Configuration

Listeners accept incoming peer connections. Each listener binds to an address and transport protocol.

## Configuration

```yaml
listeners:
  - transport: quic             # quic, h2, ws
    address: "0.0.0.0:4433"     # Bind address
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"  # Optional: require client certs
```

## Transport Types

### QUIC Listener

Best performance, UDP-based:

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### HTTP/2 Listener

TCP-based, firewall-friendly:

```yaml
listeners:
  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"              # Optional URL path
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### WebSocket Listener

Maximum compatibility:

```yaml
listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"              # Required for WebSocket
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

## Multiple Listeners

An agent can listen on multiple transports simultaneously:

```yaml
listeners:
  # Primary: QUIC for direct connections
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

  # Fallback: HTTP/2 for firewall traversal
  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

  # Alternative: WebSocket for maximum compatibility
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

## TLS Configuration

### File-Based Certificates

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### Inline PEM Certificates

For containerized deployments:

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert_pem: |
        -----BEGIN CERTIFICATE-----
        MIIBkTCB+wIJAKi...
        -----END CERTIFICATE-----
      key_pem: |
        -----BEGIN PRIVATE KEY-----
        MIIEvQIBADANBg...
        -----END PRIVATE KEY-----
```

Inline PEM takes precedence over file paths.

### Mutual TLS (mTLS)

Require clients to present valid certificates:

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"   # Require client certs signed by this CA
```

With mTLS:
- Only peers with valid certificates can connect
- Provides mutual authentication
- Recommended for production

See [TLS Configuration](tls-certificates) for details.

## Bind Address

### All Interfaces

Accept connections from anywhere:

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"    # All IPv4 interfaces
```

### Specific Interface

Bind to specific IP:

```yaml
listeners:
  - transport: quic
    address: "192.168.1.10:4433"   # Specific interface
```

### IPv6

```yaml
listeners:
  - transport: quic
    address: "[::]:4433"           # All IPv6 interfaces
```

### Localhost Only

For testing or local-only access:

```yaml
listeners:
  - transport: quic
    address: "127.0.0.1:4433"      # Localhost only
```

## Port Selection

| Transport | Default Port | Alternative Ports |
|-----------|-------------|-------------------|
| QUIC | 4433 | Any UDP port |
| HTTP/2 | 8443, 443 | Any TCP port |
| WebSocket | 443, 80 | Any TCP port |

### Firewall Considerations

```yaml
# For restrictive firewalls, use HTTPS port
listeners:
  - transport: h2
    address: "0.0.0.0:443"
    tls: ...

# Or WebSocket on standard HTTPS
listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"
    tls: ...
```

## URL Path

HTTP/2 and WebSocket support URL paths:

```yaml
listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/mesh/v1"
    tls: ...
```

Peers must use matching path:

```yaml
peers:
  - transport: h2
    address: "server.example.com:443"
    path: "/mesh/v1"
```

## Examples

### Development

```yaml
listeners:
  - transport: quic
    address: "127.0.0.1:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### Production (Multi-Transport)

```yaml
listeners:
  # QUIC for performance
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "/etc/muti-metroo/certs/agent.crt"
      key: "/etc/muti-metroo/certs/agent.key"
      client_ca: "/etc/muti-metroo/certs/ca.crt"

  # HTTP/2 for TCP fallback
  - transport: h2
    address: "0.0.0.0:443"
    path: "/mesh"
    tls:
      cert: "/etc/muti-metroo/certs/agent.crt"
      key: "/etc/muti-metroo/certs/agent.key"
      client_ca: "/etc/muti-metroo/certs/ca.crt"
```

### Docker/Kubernetes

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert_pem: "${TLS_CERT}"
      key_pem: "${TLS_KEY}"
      ca_pem: "${TLS_CA}"
```

## Troubleshooting

### Port Already in Use

```bash
# Find what's using the port
lsof -i :4433
netstat -tlnp | grep 4433

# Kill the process or choose different port
```

### Permission Denied

Ports below 1024 require root on Linux:

```bash
# Option 1: Use port > 1024
address: "0.0.0.0:4433"

# Option 2: Run as root (not recommended)
sudo ./build/muti-metroo run -c config.yaml

# Option 3: Use capabilities
sudo setcap 'cap_net_bind_service=+ep' ./build/muti-metroo
```

### Certificate Errors

```bash
# Verify certificate
./build/muti-metroo cert info ./certs/agent.crt

# Check key matches certificate
openssl x509 -noout -modulus -in agent.crt | openssl md5
openssl rsa -noout -modulus -in agent.key | openssl md5
# (should match)
```

## Related

- [Peers](peers) - Outbound connections
- [TLS Certificates](tls-certificates) - Certificate management
- [Transports](../concepts/transports) - Transport comparison
