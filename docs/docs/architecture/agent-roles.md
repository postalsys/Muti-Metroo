---
title: Agent Roles
---

# Agent Roles

An agent can serve multiple roles simultaneously.

## Ingress

**SOCKS5 proxy listener** - accepts client connections and initiates virtual streams.

- Runs SOCKS5 server on configured address
- Performs route lookup for destination
- Initiates STREAM_OPEN to exit node
- Relays data between SOCKS5 client and mesh stream

## Transit

**Relay node** - forwards streams between peers.

- Receives frames from peers
- Forwards frames to next hop
- Participates in route flooding
- No external connections

## Exit

**External access point** - opens real TCP connections.

- Advertises CIDR routes to mesh
- Receives STREAM_OPEN frames
- Opens TCP connections to destinations
- Handles DNS resolution
- Enforces route-based access control

## Multi-Role Operation

An agent can combine roles:

```yaml
# Ingress + Transit
socks5:
  enabled: true  # Ingress role

# Exit + Transit
exit:
  enabled: true  # Exit role
  routes:
    - "10.0.0.0/8"

# All three roles
socks5:
  enabled: true
exit:
  enabled: true
  routes:
    - "192.168.0.0/16"
```
