---
title: Architecture Overview
---

# Architecture Overview

Muti Metroo is designed as a modular, userspace mesh networking agent that operates without requiring root privileges.

## Core Components

- **Agent**: Main orchestrator that coordinates all subsystems
- **Transport Layer**: QUIC, HTTP/2, and WebSocket implementations
- **Protocol**: Binary frame protocol for stream and control messages
- **Routing**: Flood-based route propagation with longest-prefix match
- **Stream Manager**: Virtual stream multiplexing over peer connections
- **SOCKS5 Server**: Ingress proxy for client connections
- **Exit Handler**: External TCP connections and DNS resolution

## Design Principles

1. **Userspace Operation**: No root/kernel access required
2. **Transport Agnostic**: Support heterogeneous transports in single mesh
3. **Multi-Hop Routing**: Dynamic route discovery and propagation
4. **Resilient Connections**: Automatic reconnection with exponential backoff
5. **Resource Efficient**: Configurable limits and stream multiplexing

## Data Flow

```
Client -> SOCKS5 -> Route Lookup -> Stream -> Peer(s) -> Exit -> Destination
```

Each hop in the chain:
1. Receives frames from previous hop
2. Decodes and validates frames
3. Forwards to next hop based on routing table
4. Manages buffering and flow control

## Next Steps

- [Agent Roles](agent-roles) - Understand ingress, transit, and exit roles
- [Data Flow](data-flow) - Detailed packet flow through the mesh
- [Package Structure](packages) - Internal package organization
