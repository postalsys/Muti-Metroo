---
title: Frame Types
---

# Frame Types

Complete reference of all frame types in the Muti Metroo protocol.

## Stream Frames

### STREAM_OPEN (0x01)

Open a new virtual stream.

**Payload:**
- Destination IP (4 or 16 bytes)
- Destination port (2 bytes)
- Domain name (optional, variable length)

**Sent by:** Ingress agent  
**Received by:** Exit agent (after routing)

### STREAM_OPEN_ACK (0x02)

Acknowledge successful stream open.

**Payload:** Empty

**Sent by:** Exit agent  
**Received by:** Ingress agent

### STREAM_OPEN_ERR (0x03)

Stream open failed.

**Payload:**
- Error code (1 byte)
- Error message (variable length)

**Sent by:** Exit agent or transit agent  
**Received by:** Ingress agent

### STREAM_DATA (0x04)

Stream data payload.

**Payload:** Binary data (max 16 KB)

**Flags:**
- `FIN_WRITE`: Sender half-close
- `FIN_READ`: Receiver half-close

**Sent by:** Any agent  
**Received by:** Any agent

### STREAM_CLOSE (0x05)

Close stream.

**Payload:** Empty

**Sent by:** Any agent  
**Received by:** Any agent

### STREAM_RESET (0x06)

Reset stream with error.

**Payload:**
- Error code (1 byte)
- Error message (variable length)

**Sent by:** Any agent  
**Received by:** Any agent

## Routing Frames

### ROUTE_ADVERTISE (0x10)

Advertise CIDR routes.

**Payload:**
- Number of routes (2 bytes)
- For each route:
  - CIDR (variable)
  - Metric (1 byte)
  - TTL (4 bytes)
  - SeenBy list (variable)

**Sent by:** Exit agents and transit agents  
**Received by:** All connected peers

### ROUTE_WITHDRAW (0x11)

Withdraw CIDR routes.

**Payload:**
- Number of routes (2 bytes)
- For each route:
  - CIDR (variable)

**Sent by:** Exit agents and transit agents  
**Received by:** All connected peers

### NODE_INFO_ADVERTISE (0x12)

Advertise node metadata.

**Payload:**
- Agent ID (16 bytes)
- Display name (variable)
- Hostname (variable)
- IP address (variable)
- OS (variable)
- Architecture (variable)
- Uptime (8 bytes)

**Sent by:** All agents periodically  
**Received by:** All connected peers

## Control Frames

### PEER_HELLO (0x20)

Handshake initiation.

**Payload:**
- Protocol version (1 byte)
- Agent ID (16 bytes)

**Sent by:** Connecting peer  
**Received by:** Listening peer

### PEER_HELLO_ACK (0x21)

Handshake acknowledgment.

**Payload:**
- Protocol version (1 byte)
- Agent ID (16 bytes)

**Sent by:** Listening peer  
**Received by:** Connecting peer

### KEEPALIVE (0x22)

Connection keepalive.

**Payload:** Empty or timestamp (8 bytes)

**Sent by:** Both peers when idle  
**Received by:** Both peers

### KEEPALIVE_ACK (0x23)

Keepalive acknowledgment.

**Payload:** Empty or timestamp (8 bytes)

**Sent by:** In response to KEEPALIVE  
**Received by:** Peer that sent KEEPALIVE

### CONTROL_REQUEST (0x24)

Request metrics/status from remote agent.

**Payload:**
- Request type (1 byte)
- Request ID (4 bytes)
- Request data (variable)

**Sent by:** Any agent  
**Received by:** Target agent

### CONTROL_RESPONSE (0x25)

Response with metrics/status data.

**Payload:**
- Request ID (4 bytes)
- Response data (variable, may be chunked)

**Sent by:** Target agent  
**Received by:** Requesting agent

## Frame Flags

| Flag | Value | Description |
|------|-------|-------------|
| FIN_WRITE | 0x01 | Sender half-close (no more writes) |
| FIN_READ | 0x02 | Receiver half-close (no more reads) |
| CHUNKED | 0x04 | Payload is chunked, more frames follow |
| COMPRESSED | 0x08 | Payload is gzip compressed |
