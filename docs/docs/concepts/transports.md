---
title: Transports
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole connecting transports" style={{maxWidth: '180px'}} />
</div>

# Transport Protocols

Muti Metroo supports three transport protocols, each with different characteristics. You can mix transports within the same mesh.

## Overview

| Transport | Protocol | Port | Firewall Friendliness | Performance |
|-----------|----------|------|----------------------|-------------|
| **QUIC** | UDP | 4433 (default) | Medium | Best |
| **HTTP/2** | TCP | 443/8443 | Good | Good |
| **WebSocket** | TCP/HTTP | 443/80 | Excellent | Fair |

## QUIC Transport

QUIC (Quick UDP Internet Connections) is the recommended transport for most deployments.

### Characteristics

- **Protocol**: UDP with built-in TLS 1.3
- **Multiplexing**: Native stream multiplexing
- **Performance**: Lowest latency, best throughput
- **Connection**: Fast 0-RTT reconnection
- **Congestion**: Modern congestion control (BBR)

### When to Use

- Direct server-to-server connections
- Low-latency requirements
- High-throughput scenarios
- When UDP is not blocked

### Configuration

**Listener:**

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

**Peer connection:**

```yaml
peers:
  - id: "peer-id..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"
```

### Firewall Considerations

- Requires UDP port to be open
- May be blocked by corporate firewalls
- NAT traversal generally works well
- Some ISPs throttle or block UDP

## HTTP/2 Transport

HTTP/2 provides a TCP-based alternative with good firewall compatibility.

### Characteristics

- **Protocol**: TCP with TLS 1.3
- **Multiplexing**: HTTP/2 stream multiplexing
- **Performance**: Good, but TCP head-of-line blocking
- **Connection**: Standard TLS handshake
- **Compatibility**: Works through most firewalls

### When to Use

- Corporate environments blocking UDP
- When QUIC is not available
- Standard HTTPS infrastructure
- Load balancer compatibility needed

### Configuration

**Listener:**

```yaml
listeners:
  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"           # Optional URL path
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

**Peer connection:**

```yaml
peers:
  - id: "peer-id..."
    transport: h2
    address: "192.168.1.10:8443"
    path: "/mesh"
    tls:
      ca: "./certs/ca.crt"
```

### Firewall Considerations

- Uses standard HTTPS port (443)
- Passes through most corporate firewalls
- Compatible with HTTP proxies (without CONNECT)
- Can be hosted behind reverse proxies

## WebSocket Transport

WebSocket provides maximum compatibility, especially through HTTP proxies.

### Characteristics

- **Protocol**: HTTP upgrade to WebSocket, then framed messages
- **Multiplexing**: Application-level multiplexing over single connection
- **Performance**: Highest overhead, most latency
- **Connection**: HTTP handshake, then persistent
- **Compatibility**: Maximum - works through HTTP proxies

### When to Use

- Restrictive corporate proxies
- Browser-based clients (future)
- When HTTP/2 is blocked or problematic
- Through CDNs or WAFs

### Configuration

**Listener:**

```yaml
listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"           # URL path for WebSocket
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

**Peer connection:**

```yaml
peers:
  - id: "peer-id..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    tls:
      ca: "./certs/ca.crt"
```

**Through HTTP proxy:**

```yaml
peers:
  - id: "peer-id..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    proxy: "http://proxy.corp.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
```

### Plain WebSocket Mode (Reverse Proxy)

When deploying behind a reverse proxy that handles TLS termination (Nginx, Caddy, Apache), use the `plaintext` option to accept unencrypted WebSocket connections:

```yaml
listeners:
  - transport: ws
    address: "127.0.0.1:8080"  # Bind to localhost only
    path: "/mesh"
    plaintext: true            # No TLS - proxy handles encryption
```

**Security notes:**
- Only use behind trusted reverse proxies
- Bind to `127.0.0.1` to prevent direct external access
- mTLS is not available in this mode
- Peer authentication and end-to-end encryption still work

See [Reverse Proxy Deployment](../deployment/reverse-proxy) for complete configuration examples.

### Firewall Considerations

- Uses standard HTTP/HTTPS ports
- Works through HTTP proxies with CONNECT
- Compatible with most corporate environments
- May work through some WAFs and CDNs

## Transport Comparison

### Latency (per hop)

| Transport | LAN | WAN |
|-----------|-----|-----|
| QUIC | 1-2ms | 50-100ms |
| HTTP/2 | 2-5ms | 60-150ms |
| WebSocket | 3-10ms | 80-200ms |

### Throughput

| Transport | Single Stream | Multi-Stream |
|-----------|---------------|--------------|
| QUIC | Excellent | Excellent |
| HTTP/2 | Good | Good |
| WebSocket | Fair | Fair |

### Connection Establishment

| Transport | Initial | Reconnect |
|-----------|---------|-----------|
| QUIC | 1-RTT | 0-RTT |
| HTTP/2 | 2-RTT | 1-RTT (TLS resumption) |
| WebSocket | 2-RTT + HTTP upgrade | 2-RTT |

## Mixed Transport Deployments

You can mix transports in a single mesh:

```
                                      +----------------+
                                      |   Agent C      |
+----------------+   QUIC            |   (Transit)     |
|   Agent A      | ----------------> +----------------+
|   (Ingress)    |   (direct LAN)           |
+----------------+                          | WebSocket
        |                                   | (through proxy)
        | HTTP/2                            v
        | (corporate firewall)      +----------------+
        v                           |   Agent D      |
+----------------+                  |   (Exit)       |
|   Agent B      | <--------------- +----------------+
|   (Transit)    |   QUIC
+----------------+   (cloud)
```

### Configuration Example

Agent A (multiple transports):

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

peers:
  - id: "agent-b-id..."
    transport: h2
    address: "corporate-relay.example.com:443/mesh"
    tls:
      ca: "./certs/ca.crt"

  - id: "agent-c-id..."
    transport: quic
    address: "192.168.1.50:4433"
    tls:
      ca: "./certs/ca.crt"
```

## Transport Selection Guide

| Scenario | Recommended | Reason |
|----------|-------------|--------|
| Data center to data center | QUIC | Best performance, controlled network |
| Office to cloud | HTTP/2 or QUIC | Depends on firewall policy |
| Home user to cloud | QUIC | Most ISPs allow UDP |
| Corporate laptop to cloud | WebSocket | Works through corporate proxies |
| Through CDN/WAF | WebSocket | HTTP-based, compatible |
| High-frequency trading | QUIC | Lowest latency |
| Large file transfers | QUIC | Best throughput |

## Troubleshooting

### QUIC Connection Fails

```bash
# Check if UDP is reachable
nc -u -v target.example.com 4433

# Check firewall rules
sudo iptables -L -n | grep 4433
```

### HTTP/2 Connection Fails

```bash
# Test HTTP/2 connectivity
curl -v --http2 https://target.example.com:8443/mesh

# Check TLS
openssl s_client -connect target.example.com:8443
```

### WebSocket Connection Fails

```bash
# Test WebSocket connectivity
wscat -c wss://target.example.com:443/mesh

# Test through proxy
curl -v --proxy http://proxy:8080 https://target.example.com/mesh
```

## Next Steps

- [Routing](routing) - How routes work across transports
- [TLS Configuration](../configuration/tls-certificates) - Certificate setup
- [Troubleshooting Connectivity](../troubleshooting/connectivity) - Connection issues
