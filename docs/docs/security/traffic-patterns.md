---
title: Traffic Patterns & Detection
sidebar_position: 6
---

# Traffic Patterns & Detection

Understanding what network traffic Muti Metroo generates helps both defenders monitoring for unauthorized tunnels and operators assessing their exposure to detection.

## What Transit Observers Can See

When traffic passes through network monitoring equipment (firewalls, IDS/IPS, packet capture), observers can see connection metadata but not payload content.

### Visible to Network Observers

| Observable | Description |
|------------|-------------|
| Connection endpoints | Source and destination IP addresses |
| Ports | Default ports 4433 (QUIC), 8443 (HTTP/2), 8888 (WebSocket) |
| Protocol type | UDP for QUIC, TCP for HTTP/2 and WebSocket |
| TLS handshake patterns | Certificate exchange timing and sizes |
| Traffic volume | Bytes transferred per flow |
| Connection duration | Mesh links are long-lived |
| Timing patterns | Keepalive intervals (~5 minutes with jitter) |

### Not Visible (E2E Encrypted)

The end-to-end encryption layer (X25519 + ChaCha20-Poly1305) protects:

- **Payload content** - All stream data is encrypted
- **Frame types** - Stream open/data/close indistinguishable
- **Stream IDs** - Multiplexing is opaque
- **Actual destinations** - Only exit agent knows target IPs
- **Shell commands** - Fully encrypted
- **File contents** - Transfer data encrypted

## Transport-Specific Signatures

Each transport has distinct network characteristics that monitoring systems may detect.

### QUIC (UDP Port 4433)

QUIC traffic exhibits these patterns:

| Pattern | Detection Method |
|---------|------------------|
| Long header packets | First byte 0xC0 (Initial), 0xD0 (0-RTT), 0xE0 (Handshake) |
| Short header packets | First byte 0x40-0x5F (1-RTT encrypted data) |
| Version field | Bytes 1-4 contain 0x00000001 for QUIC v1 |
| Large initial packets | Initial packets typically >1200 bytes |
| UDP to non-standard port | Port 4433 is not commonly used |

**Detection indicators:**
- UDP traffic to port 4433 with packets >100 bytes
- QUIC Initial packet patterns (0xC0 first byte + version)
- Sustained bidirectional UDP flows

### HTTP/2 (TCP Port 8443)

HTTP/2 transport uses TLS 1.3:

| Pattern | Detection Method |
|---------|------------------|
| TLS record layer | First bytes 0x16 0x03 (handshake record) |
| ALPN negotiation | "muti-metroo/1" or "h2" in ClientHello |
| Certificate exchange | Distinctive packet sizes during handshake |
| Long-lived connections | Sessions persist for mesh lifetime |

**Detection indicators:**
- TLS handshake to port 8443
- Non-standard ALPN values in TLS ClientHello
- Persistent TCP connections with periodic small packets

### WebSocket (TCP Port 8888)

WebSocket transport wraps the mesh protocol:

| Pattern | Detection Method |
|---------|------------------|
| HTTP Upgrade | "Upgrade: websocket" header |
| Subprotocol | "Sec-WebSocket-Protocol: muti-metroo/1" |
| TLS wrapper | Same TLS patterns as HTTP/2 |
| Path | Connections to /mesh endpoint |

**Detection indicators:**
- TLS handshake to port 8888
- WebSocket upgrade with custom subprotocol
- Sustained bidirectional traffic after upgrade

## Behavioral Patterns

Beyond protocol-specific signatures, mesh traffic exhibits behavioral patterns:

### Connection Patterns

- **Long-lived connections** - Mesh links persist indefinitely
- **Keepalive timing** - ~5 minute intervals with 20% jitter
- **Reconnection behavior** - Exponential backoff (1s to 60s) on disconnect
- **Multiple connections** - Mesh topology creates multiple peer links from single source

### Traffic Patterns

- **Burst transfers** - File uploads/downloads create large packet bursts
- **Bidirectional small packets** - Shell sessions and keepalives
- **Stream multiplexing** - Multiple logical streams over single connection
- **Consistent packet sizes** - 16KB maximum frame payload

## Detection Effectiveness Summary

Based on traffic analysis lab testing with Suricata 8.0.3:

| Transport | Alerts Generated | Primary Detection Method |
|-----------|------------------|-------------------------|
| QUIC | High volume | UDP packet patterns, port 4433 |
| HTTP/2 | Moderate | TLS handshake, port 8443 |
| WebSocket | Moderate | TLS handshake, port 8888 |

**Most effective detection methods:**

1. **Port-based detection** - Non-standard ports (4433, 8443, 8888)
2. **TLS fingerprinting** - ALPN values, JA3/JA4 hashes
3. **Behavioral analysis** - Long-lived connections, keepalive patterns
4. **Protocol parsing** - QUIC packet type identification

**Least effective detection methods:**

1. **Payload inspection** - E2E encryption prevents content analysis
2. **Deep packet inspection** - Encrypted payloads appear random
3. **Application identification** - Standard TLS/QUIC appears legitimate

## Reducing Detection Risk

To minimize network signature visibility:

### Protocol Configuration

```yaml
protocol:
  alpn: ""           # Disable custom ALPN
  http_header: ""    # Disable custom HTTP header
  ws_subprotocol: "" # Disable custom WebSocket subprotocol
```

### Port Selection

Use standard ports that blend with normal traffic:

```yaml
listeners:
  - transport: h2
    address: ":443"  # Standard HTTPS port
```

### Timing Adjustments

Increase keepalive jitter to reduce timing fingerprint:

```yaml
connections:
  keepalive_jitter: 0.5  # 50% timing variance
```

### Transport Selection

- **HTTP/2 on port 443** - Appears as normal HTTPS
- **WebSocket on port 443** - Common for web applications
- **QUIC** - Increasingly common, harder to block

## Traffic Analysis Lab

The detection patterns documented here were validated using a dedicated traffic analysis lab.

### Lab Configuration

| Component | Details |
|-----------|---------|
| IDS/IPS | Suricata 8.0.3 |
| Capture method | AF_PACKET on transit node |
| Network mode | Sidecar container sharing network namespace |
| Test traffic | SOCKS5 proxy, file transfers, shell sessions |

### Test Methodology

1. Three-agent mesh: ingress -> transit (monitored) -> exit
2. Suricata sidecar captures all transit traffic
3. Tests run for each transport type separately
4. Custom Suricata rules detect Muti Metroo patterns

### Detection Rules Tested

| Rule Category | Count | Description |
|---------------|-------|-------------|
| QUIC patterns | 7 | Packet headers, version, session tracking |
| TLS/HTTP2 patterns | 4 | Handshake, ALPN, session tracking |
| WebSocket patterns | 4 | Upgrade, subprotocol, TLS |
| Behavioral | 3 | Large transfers, keepalives |
| Multi-port | 1 | TLS on any mesh port |
| Topology | 2 | Multiple connections from same source |

### Lab Availability

The traffic analysis lab is available in the repository at `traffic-analysis/`:

```bash
cd traffic-analysis
./scripts/setup.sh        # Initialize lab
docker compose up -d      # Start services
./scripts/run-tests.sh quic  # Run QUIC tests
./scripts/generate-report.sh # Generate report
```

## Recommendations

### For Network Defenders

1. **Monitor non-standard ports** - Alert on TLS/QUIC to ports 4433, 8443, 8888
2. **TLS fingerprinting** - Use JA3/JA4 to identify unusual TLS clients
3. **Connection duration** - Flag connections lasting >1 hour
4. **Traffic correlation** - Look for mesh topology patterns (A->B->C)
5. **QUIC inspection** - Parse QUIC headers for version and packet types

### For Operators

1. **Use standard ports** - 443 blends with normal HTTPS
2. **Disable protocol identifiers** - Empty ALPN, headers, subprotocols
3. **Domain fronting** - Route through CDN where applicable
4. **Traffic shaping** - Vary packet sizes and timing
5. **Decoy traffic** - Mix with legitimate application traffic

## Related Topics

- [E2E Encryption](/security/e2e-encryption) - How payload encryption works
- [TLS/mTLS](/security/tls-mtls) - Transport layer security
- [Transports](/concepts/transports) - Available transport protocols
- [Best Practices](/security/best-practices) - Security hardening
