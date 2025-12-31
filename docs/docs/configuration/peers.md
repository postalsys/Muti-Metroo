---
title: Peers
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring peers" style={{maxWidth: '180px'}} />
</div>

# Peer Configuration

Peers define outbound connections to other agents in the mesh.

## Configuration

```yaml
peers:
  - id: "abc123def456789012345678901234ab"   # Expected peer Agent ID
    transport: quic                           # quic, h2, ws
    address: "192.168.1.10:4433"             # Peer address
    tls:
      ca: "./certs/ca.crt"                   # CA certificate
```

## Peer Options

### Basic Peer

```yaml
peers:
  - id: "abc123def456789012345678901234ab"
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"
```

### Full Options

```yaml
peers:
  - id: "abc123def456789012345678901234ab"
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"      # Client certificate (for mTLS)
      key: "./certs/client.key"       # Client key (for mTLS)
    reconnect:
      initial_delay: 1s
      max_delay: 60s
      multiplier: 2.0
      jitter: 0.2
```

## Peer ID

The `id` field specifies the expected Agent ID of the peer:

```yaml
peers:
  - id: "abc123def456789012345678901234ab"
```

This provides:
- **Authentication**: Verify you are connecting to the right agent
- **Security**: Prevent man-in-the-middle attacks
- **Routing**: Identify peer for route lookup

### Getting Peer ID

From the peer's agent:

```bash
# From file
cat /path/to/peer/data/agent_id

# From API
curl http://peer-host:8080/healthz | jq -r '.agent_id'

# From logs
# Look for: Agent ID: abc123...
```

## Transport Types

### QUIC

```yaml
peers:
  - id: "..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"
```

### HTTP/2

```yaml
peers:
  - id: "..."
    transport: h2
    address: "192.168.1.10:8443"
    path: "/mesh"                    # Must match listener path
    tls:
      ca: "./certs/ca.crt"
```

### WebSocket

```yaml
peers:
  - id: "..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    tls:
      ca: "./certs/ca.crt"
```

### WebSocket Through Proxy

```yaml
peers:
  - id: "..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    proxy: "http://proxy.corp.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
    tls:
      ca: "./certs/ca.crt"
```

## TLS Configuration

### Server CA Only

Validate server certificate:

```yaml
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
```

### Mutual TLS (mTLS)

Present client certificate:

```yaml
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"
      key: "./certs/client.key"
```

### Inline Certificates

```yaml
peers:
  - id: "..."
    tls:
      ca_pem: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
      cert_pem: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
      key_pem: |
        -----BEGIN PRIVATE KEY-----
        ...
        -----END PRIVATE KEY-----
```

## Reconnection

Configure automatic reconnection behavior:

```yaml
peers:
  - id: "..."
    reconnect:
      initial_delay: 1s         # First retry delay
      max_delay: 60s            # Maximum retry delay
      multiplier: 2.0           # Exponential backoff multiplier
      jitter: 0.2               # 20% random jitter
      max_retries: 0            # 0 = infinite retries
```

### Reconnection Algorithm

```
delay = min(initial_delay * multiplier^attempt, max_delay) * (1 + random(jitter))
```

Example with defaults:
- Attempt 1: ~1s
- Attempt 2: ~2s
- Attempt 3: ~4s
- Attempt 4: ~8s
- ... (caps at 60s)

### Disabling Reconnection

```yaml
peers:
  - id: "..."
    reconnect:
      max_retries: 1            # Only try once
```

## Multiple Peers

Connect to multiple agents:

```yaml
peers:
  # Direct QUIC to local agent
  - id: "agent-local-id..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"

  # HTTP/2 to cloud relay
  - id: "agent-cloud-id..."
    transport: h2
    address: "relay.cloud.com:443"
    path: "/mesh"
    tls:
      ca: "./certs/cloud-ca.crt"

  # WebSocket through proxy to remote site
  - id: "agent-remote-id..."
    transport: ws
    address: "wss://remote.site.com:443/mesh"
    proxy: "http://proxy:8080"
    tls:
      ca: "./certs/remote-ca.crt"
```

## Address Formats

### IPv4

```yaml
address: "192.168.1.10:4433"
```

### IPv6

```yaml
address: "[2001:db8::1]:4433"
```

### Hostname

```yaml
address: "agent.example.com:4433"
```

### With Path (HTTP/2, WebSocket)

```yaml
address: "agent.example.com:443"
path: "/mesh"

# Or full URL for WebSocket
address: "wss://agent.example.com:443/mesh"
```

## Environment Variables

```yaml
peers:
  - id: "${PEER_ID}"
    transport: "${PEER_TRANSPORT:-quic}"
    address: "${PEER_ADDR}"
    tls:
      ca: "${PEER_CA:-./certs/ca.crt}"
```

## Examples

### Two-Agent Setup

Agent A connects to Agent B:

```yaml
# Agent A config
peers:
  - id: "bbbb2222..."    # Agent B's ID
    transport: quic
    address: "192.168.1.20:4433"
    tls:
      ca: "./certs/ca.crt"
```

Agent B (listener only, no peers needed):

```yaml
# Agent B config
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### Hub and Spoke

Central hub with multiple spokes:

```yaml
# Hub config (no outbound peers, just listeners)
listeners:
  - transport: quic
    address: "0.0.0.0:4433"

# Spoke configs
peers:
  - id: "hub-agent-id..."
    transport: quic
    address: "hub.example.com:4433"
    tls:
      ca: "./certs/ca.crt"
```

### Full Mesh

Each agent connects to all others:

```yaml
# Agent A
peers:
  - id: "agent-b-id..."
    address: "192.168.1.20:4433"
  - id: "agent-c-id..."
    address: "192.168.1.30:4433"

# Agent B
peers:
  - id: "agent-a-id..."
    address: "192.168.1.10:4433"
  - id: "agent-c-id..."
    address: "192.168.1.30:4433"

# Agent C
peers:
  - id: "agent-a-id..."
    address: "192.168.1.10:4433"
  - id: "agent-b-id..."
    address: "192.168.1.20:4433"
```

## Troubleshooting

### Connection Failed

```bash
# Check peer is reachable
nc -zv 192.168.1.10 4433

# Check DNS resolution
dig agent.example.com

# Check with debug logging
muti-metroo run -c config.yaml --log-level debug
```

### Certificate Errors

```bash
# Verify CA certificate
openssl x509 -in ./certs/ca.crt -text -noout

# Test TLS connection
openssl s_client -connect 192.168.1.10:4433 -CAfile ./certs/ca.crt
```

### Wrong Peer ID

```
ERROR  Peer ID mismatch: expected abc123..., got def456...
```

Update the `id` field to match the actual peer Agent ID.

## Related

- [Listeners](listeners) - Accept incoming connections
- [TLS Certificates](tls-certificates) - Certificate setup
- [Transports](../concepts/transports) - Transport details
