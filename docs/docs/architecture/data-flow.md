---
title: Data Flow
---

# Data Flow

How packets flow through the Muti Metroo mesh.

## Stream Open Sequence

1. **Client connects to SOCKS5 proxy**
   - Client sends SOCKS5 CONNECT request
   - Ingress agent extracts destination IP/port

2. **Route lookup**
   - Longest-prefix match in routing table
   - Returns next-hop peer ID

3. **STREAM_OPEN frame sent**
   - Sent through peer chain to exit agent
   - Contains destination IP/port

4. **Exit agent opens TCP connection**
   - Validates against allowed routes
   - Opens connection to destination
   - Sends STREAM_OPEN_ACK back

5. **Bidirectional relay begins**
   - STREAM_DATA frames flow both directions
   - Each hop buffers and forwards

## Frame Relay

Each agent in the path:

```
Receive -> Decode -> Route Lookup -> Forward -> Encode -> Send
```

- **Receive**: Read frame from peer connection
- **Decode**: Parse 14-byte header + payload
- **Route Lookup**: Determine next hop for stream
- **Forward**: Pass to appropriate peer or local handler
- **Encode**: Serialize frame for transmission
- **Send**: Write to peer connection

## Connection Close

Half-close and full-close supported:

1. **FIN_WRITE**: One direction closed (half-close)
2. **FIN_READ**: Other direction closed
3. **STREAM_CLOSE**: Full close notification

## Route Propagation

Routes flood through the mesh:

1. **Exit agent advertises route**
   - ROUTE_ADVERTISE frame with CIDR
2. **Peers forward advertisement**
   - Increment hop count
   - Add to SeenBy list
3. **Loop prevention**
   - Don't forward to agents in SeenBy
   - TTL/max hops enforcement
