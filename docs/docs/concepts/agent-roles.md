---
title: Agent Roles
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole explaining roles" style={{maxWidth: '180px'}} />
</div>

# Agent Roles

Every Muti Metroo agent can serve one or more roles simultaneously. Understanding these roles is essential for designing your mesh topology.

## Overview

| Role | Description | Typical Use |
|------|-------------|-------------|
| **Ingress** | Accepts client connections via SOCKS5 | User-facing proxy endpoint |
| **Transit** | Relays traffic between other agents | Network bridge, VPN gateway |
| **Exit** | Opens connections to external destinations | Internet gateway, private network access |

## Ingress Role

An **ingress agent** accepts client connections and initiates streams into the mesh.

### Responsibilities

- Run SOCKS5 server on configured address
- Authenticate clients (if enabled)
- Perform route lookup for destinations
- Open streams to the appropriate exit node
- Relay data between SOCKS5 client and mesh stream

### Configuration

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
  max_connections: 1000
```

### Typical Deployment

- **User workstations**: Local SOCKS5 proxy
- **Edge servers**: Remote access gateway
- **Cloud instances**: Entry point for distributed team

### Example: Ingress-Only Agent

```yaml
agent:
  display_name: "Ingress Gateway"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

peers:
  - id: "transit-agent-id..."
    transport: quic
    address: "relay.example.com:4433"
    tls:
      ca: "./certs/ca.crt"

socks5:
  enabled: true
  address: "0.0.0.0:1080"
  auth:
    enabled: true
    users:
      - username: "team"
        password_hash: "$2a$10$..."

# No exit configuration - pure ingress
```

## Transit Role

A **transit agent** relays traffic between other agents without initiating or terminating connections.

### Responsibilities

- Receive data from connected peers
- Forward data to next hop based on routing
- Propagate route advertisements through the mesh
- Provide bridging between network segments

### Configuration

Transit role is implicit - any agent that connects to multiple peers becomes a transit node:

```yaml
# No specific configuration needed
# Just connect to multiple peers
peers:
  - id: "peer-a-id..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/ca.crt"

  - id: "peer-b-id..."
    transport: quic
    address: "192.168.1.20:4433"
    tls:
      ca: "./certs/ca.crt"
```

### Typical Deployment

- **Cloud relay**: Bridge between on-premise networks
- **DMZ server**: Connect internal and external networks
- **Geographic hop**: Reduce latency across regions

### Example: Transit-Only Agent

```yaml
agent:
  display_name: "Cloud Transit"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

peers:
  - id: "site-a-agent..."
    transport: quic
    address: "site-a.example.com:4433"
    tls:
      ca: "./certs/ca.crt"

  - id: "site-b-agent..."
    transport: quic
    address: "site-b.example.com:4433"
    tls:
      ca: "./certs/ca.crt"

# No socks5 or exit - pure transit
http:
  enabled: true
  address: ":8080"
```

## Exit Role

An **exit agent** opens real TCP connections to external destinations.

### Responsibilities

- Advertise CIDR routes to the mesh
- Accept stream open requests from ingress agents
- Validate destination against allowed routes
- Open TCP connections to destinations
- Handle DNS resolution for domain names
- Enforce access control

### Configuration

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"        # Private network
    - "192.168.0.0/16"    # Another private network
    - "0.0.0.0/0"         # Default route (internet)
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

### Typical Deployment

- **Internet gateway**: Default route for all traffic
- **Private network access**: Routes to internal resources
- **Service endpoint**: Access to specific services/CIDRs

### Example: Exit-Only Agent

```yaml
agent:
  display_name: "Exit Gateway"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"        # Company internal network
  dns:
    servers:
      - "10.0.0.1:53"     # Internal DNS
    timeout: 5s

# No socks5 - pure exit
http:
  enabled: true
  address: ":8080"
```

## Combined Roles

Agents can combine multiple roles. Common patterns:

### Ingress + Transit

Acts as entry point and relay:

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"

peers:
  - id: "remote-exit-id..."
    address: "remote.example.com:4433"
```

### Transit + Exit

Relays traffic and provides exit for specific routes:

```yaml
peers:
  - id: "ingress-id..."
    address: "ingress.example.com:4433"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
```

### All Roles (Standalone)

Single agent that does everything:

```yaml
agent:
  display_name: "All-in-One"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
```

## Role Selection Guide

| Scenario | Recommended Roles |
|----------|------------------|
| Local proxy for one user | Ingress + Exit (standalone) |
| Team proxy with cloud exit | Ingress (local) + Exit (cloud) |
| Multi-site connectivity | Transit (cloud) + Exit (each site) |
| Internet gateway for office | Ingress (office) + Exit (DMZ) |
| Geo-distributed access | Multiple Transit + regional Exits |

## Role Visualization

The web dashboard (`/ui/`) shows each agent's active roles:

```
+-------------------+
|  Agent: gateway   |
|  Roles: I T E     |
|  Peers: 3         |
|  Routes: 5        |
+-------------------+

I = Ingress (SOCKS5 enabled)
T = Transit (has peer connections)
E = Exit (has exit routes)
```

## Best Practices

1. **Minimize exit points**: Fewer exits are easier to monitor and secure
2. **Place transit in DMZ**: Transit agents don't need to access sensitive resources
3. **Use specific routes**: Avoid `0.0.0.0/0` on exits unless necessary
4. **Separate concerns**: Don't combine ingress and exit on the same agent in production
5. **Monitor all roles**: Enable HTTP API for metrics on every agent

## Next Steps

- [Transports](transports) - Choose the right transport for each role
- [Routing](routing) - How routes propagate between roles
- [Security](../security/overview) - Secure each role appropriately
