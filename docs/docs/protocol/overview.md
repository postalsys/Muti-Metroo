---
title: Protocol Overview
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-thinking.png" alt="Mole designing protocol" style={{maxWidth: '180px'}} />
</div>

# Protocol Overview

Muti Metroo uses a binary frame-based protocol for all peer communication.

## Protocol Version

Current version: **0x01**

## Frame Structure

Every frame has a 14-byte header followed by variable-length payload:

```
+----------------+----------------+----------------+----------------+
| Type (1 byte)  | Flags (1 byte) |   StreamID (4 bytes)            |
+----------------+----------------+----------------+----------------+
|                    Payload Length (4 bytes)                      |
+----------------+----------------+----------------+----------------+
|                    Reserved (4 bytes)                            |
+----------------+----------------+----------------+----------------+
|                    Payload (0-16384 bytes)                       |
+------------------------------------------------------------------|
```

## Frame Types

See [Frame Types](frames) for complete list.

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| Protocol Version | 0x01 | Current protocol version |
| Max Frame Payload | 16 KB | Maximum payload size |
| Header Size | 14 bytes | Fixed header size |
| Control Stream ID | 0 | Reserved for control channel |

## Stream ID Allocation

- **Connection initiator (dialer)**: ODD IDs (1, 3, 5, ...)
- **Connection acceptor (listener)**: EVEN IDs (2, 4, 6, ...)
- **StreamID 0**: Reserved for control messages

## Connection Lifecycle

1. **Handshake**: PEER_HELLO / PEER_HELLO_ACK exchange
2. **Operation**: Stream and control frames
3. **Keepalive**: KEEPALIVE / KEEPALIVE_ACK when idle
4. **Disconnect**: Connection close or timeout

## Timeouts

| Operation | Timeout |
|-----------|---------|
| Handshake | 10s |
| Stream open | 30s |
| Keepalive interval | 30s |
| Keepalive timeout | 90s |
| Idle stream | 5m |

See [Limits](limits) for complete limits reference.
