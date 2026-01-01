---
title: Frame Types
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-thinking.png" alt="Mole analyzing frames" style={{maxWidth: '180px'}} />
</div>

# Frame Types

Complete reference of all frame types in the Muti Metroo protocol.

## Frame Header

All frames share a common 14-byte header:

| Field | Size | Description |
|-------|------|-------------|
| Type | 1 byte | Frame type (see below) |
| Flags | 1 byte | Frame flags |
| Length | 4 bytes | Payload length (big-endian) |
| StreamID | 8 bytes | Stream identifier (big-endian) |

## Stream Frames

### STREAM_OPEN (0x01)

Open a new virtual stream.

**Payload:**
- Request ID (8 bytes)
- Address type (1 byte): 0x01 = IPv4, 0x03 = domain, 0x04 = IPv6
- Address (4 bytes for IPv4, 16 for IPv6, or 1-byte length + domain string)
- Destination port (2 bytes)
- TTL (1 byte)
- Path length (1 byte)
- Remaining path (N * 16 bytes, where N is path length)

**Sent by:** Ingress agent
**Received by:** Exit agent (after routing)

### STREAM_OPEN_ACK (0x02)

Acknowledge successful stream open.

**Payload:**
- Request ID (8 bytes)
- Bound address type (1 byte)
- Bound address (4 or 16 bytes)
- Bound port (2 bytes)

**Sent by:** Exit agent
**Received by:** Ingress agent

### STREAM_OPEN_ERR (0x03)

Stream open failed.

**Payload:**
- Request ID (8 bytes)
- Error code (2 bytes, big-endian)
- Message length (1 byte)
- Error message (variable length, max 255 chars)

**Sent by:** Exit agent or transit agent
**Received by:** Ingress agent

### STREAM_DATA (0x04)

Stream data payload.

**Payload:** Binary data (max 16 KB)

**Flags:**
- `FIN_WRITE` (0x01): Sender half-close (no more writes)
- `FIN_READ` (0x02): Receiver half-close (no more reads)

**Sent by:** Any agent
**Received by:** Any agent

### STREAM_CLOSE (0x05)

Graceful stream close.

**Payload:** Empty

**Sent by:** Any agent
**Received by:** Any agent

### STREAM_RESET (0x06)

Abort stream with error.

**Payload:**
- Error code (2 bytes, big-endian)

**Sent by:** Any agent
**Received by:** Any agent

## Routing Frames

### ROUTE_ADVERTISE (0x10)

Advertise CIDR routes.

**Payload:**
- Origin agent ID (16 bytes)
- Display name length (1 byte) + display name (variable)
- Sequence number (8 bytes)
- Route count (1 byte)
- For each route:
  - Address family (1 byte): 0x01 = IPv4, 0x02 = IPv6
  - Prefix length (1 byte)
  - Prefix (4 or 16 bytes)
  - Metric (2 bytes, big-endian)
- Path length (1 byte) + path agent IDs (N * 16 bytes)
- SeenBy length (1 byte) + seen agent IDs (N * 16 bytes)

**Sent by:** Exit agents and transit agents
**Received by:** All connected peers

### ROUTE_WITHDRAW (0x11)

Withdraw CIDR routes.

**Payload:**
- Origin agent ID (16 bytes)
- Sequence number (8 bytes)
- Route count (1 byte)
- For each route:
  - Address family (1 byte)
  - Prefix length (1 byte)
  - Prefix (4 or 16 bytes)
  - Metric (2 bytes)
- SeenBy length (1 byte) + seen agent IDs (N * 16 bytes)

**Sent by:** Exit agents and transit agents
**Received by:** All connected peers

### NODE_INFO_ADVERTISE (0x12)

Advertise node metadata.

**Payload:**
- Origin agent ID (16 bytes)
- Sequence number (8 bytes)
- Display name length (1 byte) + display name
- Hostname length (1 byte) + hostname
- OS length (1 byte) + OS
- Arch length (1 byte) + architecture
- Version length (1 byte) + version
- Start time (8 bytes, Unix timestamp)
- IP count (1 byte) + IP addresses (each: length + string)
- SeenBy length (1 byte) + seen agent IDs (N * 16 bytes)
- Peer count (1 byte) + peer connection info (each: 16 + 1+transport + 8 + 1)

**Sent by:** All agents periodically
**Received by:** All connected peers

## Control Frames

### PEER_HELLO (0x20)

Handshake initiation.

**Payload:**
- Protocol version (2 bytes, big-endian)
- Agent ID (16 bytes)
- Timestamp (8 bytes, Unix nanoseconds)
- Display name length (1 byte) + display name
- Capabilities count (1 byte) + capabilities (each: length + string)

**Sent by:** Connecting peer
**Received by:** Listening peer

### PEER_HELLO_ACK (0x21)

Handshake acknowledgment.

**Payload:** Same as PEER_HELLO

**Sent by:** Listening peer
**Received by:** Connecting peer

### KEEPALIVE (0x22)

Connection keepalive.

**Payload:**
- Timestamp (8 bytes, Unix nanoseconds)

**Sent by:** Both peers when idle
**Received by:** Both peers

### KEEPALIVE_ACK (0x23)

Keepalive acknowledgment.

**Payload:**
- Timestamp (8 bytes, echoed from KEEPALIVE)

**Sent by:** In response to KEEPALIVE
**Received by:** Peer that sent KEEPALIVE

### CONTROL_REQUEST (0x24)

Request metrics/status from remote agent.

**Payload:**
- Request ID (8 bytes)
- Control type (1 byte):
  - 0x01 = Metrics
  - 0x02 = Status
  - 0x03 = Peers
  - 0x04 = Routes
  - 0x05 = RPC
- Target agent ID (16 bytes)
- Path length (1 byte) + path agent IDs (N * 16 bytes)
- Data length (4 bytes) + request data (variable)

**Sent by:** Any agent
**Received by:** Target agent

### CONTROL_RESPONSE (0x25)

Response with metrics/status data.

**Payload:**
- Request ID (8 bytes)
- Control type (1 byte)
- Success (1 byte, 0 or 1)
- Data length (2 bytes) + response data (variable)

**Sent by:** Target agent
**Received by:** Requesting agent

## Frame Flags

| Flag | Value | Description |
|------|-------|-------------|
| FIN_WRITE | 0x01 | Sender half-close (no more writes) |
| FIN_READ | 0x02 | Receiver half-close (no more reads) |

## Error Codes

Used in STREAM_OPEN_ERR and STREAM_RESET frames:

| Code | Name | Description |
|------|------|-------------|
| 1 | NO_ROUTE | No route to destination |
| 2 | CONNECTION_REFUSED | Connection refused by target |
| 3 | CONNECTION_TIMEOUT | Connection attempt timed out |
| 4 | TTL_EXCEEDED | TTL reached zero before reaching exit |
| 5 | HOST_UNREACHABLE | Host unreachable |
| 6 | NETWORK_UNREACHABLE | Network unreachable |
| 7 | DNS_ERROR | DNS resolution failed |
| 8 | EXIT_DISABLED | Exit handler not enabled |
| 9 | RESOURCE_LIMIT | Resource limit exceeded |
| 10 | CONNECTION_LIMIT | Connection limit exceeded |
| 11 | NOT_ALLOWED | Connection not allowed by policy |
| 12 | FILE_TRANSFER_DENIED | File transfer denied |
| 13 | AUTH_REQUIRED | Authentication required |
| 14 | PATH_NOT_ALLOWED | Path not in allowed list |
| 15 | FILE_TOO_LARGE | File exceeds size limit |
| 16 | FILE_NOT_FOUND | File not found |
| 17 | WRITE_FAILED | Write operation failed |
