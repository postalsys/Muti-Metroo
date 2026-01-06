---
title: Transport Selection
sidebar_label: Transport Selection
sidebar_position: 3
---

# Transport Selection

Choose transports based on target environment and evasion requirements.

## QUIC (UDP)

**Best for:** High performance, NAT traversal, mobile networks

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:443"  # Use common port
```

**Considerations:**
- UDP-based, may trigger alerts on networks expecting TCP-only
- Excellent performance with native multiplexing
- Works well through NAT without port forwarding
- Some enterprise firewalls block non-TCP/80/443

## HTTP/2 (TCP)

**Best for:** Corporate environments, blending with HTTPS traffic

```yaml
listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/v2/stream"  # Realistic API path
```

**Considerations:**
- Indistinguishable from normal HTTPS on wire
- Works through TLS-inspecting proxies (with valid certs)
- Single TCP connection with frame multiplexing
- Path should match cover story (e.g., `/api/`, `/ws/`, `/connect`)

## WebSocket (TCP)

**Best for:** Maximum compatibility, HTTP proxy traversal, CDN fronting

```yaml
listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/socket.io/"  # Common WebSocket path

peers:
  - transport: ws
    address: "wss://cdn-endpoint.example.com:443/api/realtime"
    proxy: "http://corporate-proxy.internal:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
```

**Considerations:**
- **HTTP proxy support** with authentication
- Works through corporate proxies and CDNs
- Can use domain fronting techniques
- Upgrade headers may be logged

## Transport Comparison Matrix

| Factor | QUIC | HTTP/2 | WebSocket |
|--------|------|--------|-----------|
| Protocol | UDP | TCP | TCP |
| Default Port | 443 | 443 | 443/80 |
| Proxy Support | No | Limited | **Yes** |
| CDN Fronting | No | Yes | **Yes** |
| Corporate Firewall | Medium | High | **High** |
| Performance | **Best** | Good | Good |
| NAT Traversal | **Best** | Good | Good |
