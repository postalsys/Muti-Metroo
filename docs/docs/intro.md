---
slug: /
title: Introduction
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-surfacing.png" alt="Muti Metroo Mole" style={{maxWidth: '200px'}} />
</div>

# Muti Metroo

**Muti Metroo** is a userspace mesh networking agent that creates virtual TCP tunnels across heterogeneous transport layers. It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

## What is Muti Metroo?

Muti Metroo allows you to build flexible, resilient mesh networks where traffic can flow through multiple intermediate nodes to reach its destination. Think of it as building your own private network overlay that works across different network segments, firewalls, and transport protocols.

```
                                 +------------------+
                                 |   Internet       |
                                 +--------+---------+
                                          |
+-------------+     +-------------+     +-+----------+
|   Client    |     |   Agent A   |     |  Agent C   |
|  (Browser)  +---->+  (Ingress)  +---->+  (Exit)    |
+-------------+     +------+------+     +------------+
   SOCKS5                  |
   Proxy                   |
                     +-----v------+
                     |  Agent B   |
                     | (Transit)  |
                     +------------+
```

## Key Features

| Feature | Description |
|---------|-------------|
| **Multiple Transports** | QUIC/TLS 1.3, HTTP/2, and WebSocket - mix protocols in a single mesh |
| **SOCKS5 Proxy** | Accept client connections with optional authentication |
| **CIDR-Based Routing** | Advertise network routes and handle DNS at exit nodes |
| **Multi-Hop Paths** | Traffic automatically finds its way through the mesh |
| **Stream Multiplexing** | Multiple virtual streams over single connections |
| **File Transfer** | Upload/download files and directories across the mesh |
| **Remote Execution** | Execute commands on remote agents (RPC) |
| **Web Dashboard** | Visual topology with metro map visualization |
| **No Root Required** | Runs entirely in userspace |

## Use Cases

### Corporate Network Access

Provide secure access to internal resources through multi-hop SOCKS5 proxy chains, bypassing network segmentation without VPN infrastructure.

```
Employee Laptop --[SOCKS5]--> Cloud Agent --[QUIC]--> Office Agent --[TCP]--> Internal Server
```

### Multi-Site Connectivity

Connect multiple office locations through a mesh of agents, enabling seamless access to resources across sites.

```
Site A Agent <--[HTTP/2]--> Cloud Relay <--[WebSocket]--> Site B Agent
     |                                                          |
Private Network A                                     Private Network B
```

### Resilient Remote Access

Maintain connectivity through redundant paths with automatic failover and reconnection.

### Development and Testing

Create complex network topologies for testing distributed applications without physical infrastructure.

## How It Works

1. **Agents** connect to form a mesh network, each potentially serving as ingress, transit, or exit
2. **Routes** are advertised through the mesh using flood-based propagation
3. **Clients** connect via SOCKS5 proxy on an ingress agent
4. **Traffic** flows through the mesh following the best route to the exit agent
5. **Exit agents** open real TCP connections to destination servers

## Quick Start

Get up and running in minutes:

```bash
# Clone and build
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4
make build

# Run interactive setup wizard
./build/muti-metroo setup
```

The wizard guides you through configuring your first agent, generating TLS certificates, and starting the mesh.

## Documentation Overview

| Section | Description |
|---------|-------------|
| [Getting Started](getting-started/overview) | Installation, setup, and your first mesh |
| [Core Concepts](concepts/architecture) | Architecture, roles, transports, and routing |
| [Configuration](configuration/overview) | Complete configuration reference |
| [Features](features/socks5-proxy) | SOCKS5, exit routing, file transfer, RPC |
| [Deployment](deployment/scenarios) | Docker, Kubernetes, and production deployment |
| [Security](security/overview) | TLS, authentication, and best practices |
| [CLI Reference](cli/overview) | Command-line interface documentation |
| [HTTP API](api/overview) | REST API for monitoring and management |
| [Protocol](protocol/overview) | Wire protocol and internals |
| [Troubleshooting](troubleshooting/common-issues) | Common issues and FAQ |

## Next Steps

- **New to Muti Metroo?** Start with [Getting Started](getting-started/overview)
- **Want to understand the architecture?** Read [Core Concepts](concepts/architecture)
- **Ready to deploy?** Check out [Deployment Scenarios](deployment/scenarios)
- **Need help?** Visit [Troubleshooting](troubleshooting/common-issues)
