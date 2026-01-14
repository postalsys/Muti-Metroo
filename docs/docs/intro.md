---
title: Introduction
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-surfacing.png" alt="Muti Metroo Mole" style={{maxWidth: '200px'}} />
</div>

# Muti Metroo

**Your private metro system for network traffic** - tunnel through firewalls and bridge private networks with encrypted relay chains.

## What is Muti Metroo?

Muti Metroo allows you to build flexible, resilient mesh networks where traffic can flow through multiple intermediate nodes to reach its destination. Think of it as building your own private network overlay that works across different network segments, firewalls, and transport protocols.

```mermaid
flowchart LR
    subgraph Internet
        Server[Internet]
    end

    Client[Client<br/>Browser] -->|SOCKS5| A[Agent A<br/>Ingress]
    A --> B[Agent B<br/>Transit]
    B --> C[Agent C<br/>Exit]
    C --> Server
```

## Why Muti Metroo?

| Challenge | How Muti Metroo Helps |
| --------- | --------------------- |
| **Need to reach a restricted network** | Tunnel through firewalls using HTTP/2 or WebSocket that blends with normal HTTPS |
| **Complex network topology** | Build multi-hop relay chains - traffic automatically finds its path to the exit |
| **Hard to deploy** | Single binary, no root required, no kernel modules - deploy in seconds |
| **Per-application configuration** | SOCKS5 proxy or TUN interface routes all traffic transparently |
| **Security concerns** | End-to-end encrypted - transit nodes relay data they cannot decrypt |

## Key Features

| Feature | What It Does |
| ------- | ------------ |
| **Firewall Traversal** | HTTP/2 and WebSocket transports blend with HTTPS traffic to bypass restrictive firewalls |
| **Multi-Hop Routing** | Automatic route propagation - traffic flows through chains, trees, or full mesh topologies |
| **SOCKS5 Proxy** | TCP CONNECT and UDP ASSOCIATE with optional authentication |
| **CIDR and Domain Routes** | Route by IP range or domain pattern with DNS resolution at the exit node |
| **Port Forwarding** | Expose local services through reverse tunnels - serve tools or receive callbacks from anywhere in the mesh |
| **File Transfer** | Upload/download files and directories to any agent in the mesh |
| **Remote Shell** | Execute commands on remote agents with interactive PTY support |
| **TUN Interface** | Transparent L3 routing with [Mutiauk](/mutiauk) - no per-app configuration |
| **Web Dashboard** | Visual metro map showing mesh topology and connections |
| **No Root Required** | Runs entirely in userspace as a single binary |
| **E2E Encryption** | X25519 + ChaCha20-Poly1305 - transit nodes cannot decrypt your traffic |

## Use Cases

### Corporate Network Access

Provide secure access to internal resources through multi-hop SOCKS5 proxy chains, bypassing network segmentation without VPN infrastructure.

```mermaid
flowchart LR
    Laptop[Employee Laptop] -->|SOCKS5| Cloud[Cloud Agent]
    Cloud -->|QUIC| Office[Office Agent]
    Office -->|TCP| Internal[Internal Server]
```

### Multi-Site Connectivity

Connect multiple office locations through a mesh of agents, enabling seamless access to resources across sites.

```mermaid
flowchart LR
    subgraph SiteA[Site A]
        NetA[Private Network A] --- AgentA[Site A Agent]
    end

    subgraph SiteB[Site B]
        AgentB[Site B Agent] --- NetB[Private Network B]
    end

    AgentA <-->|HTTP/2| Cloud[Cloud Relay]
    Cloud <-->|WebSocket| AgentB
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
5. **Exit agents** open real TCP connections or relay UDP datagrams to destinations

## Quick Start

Get up and running in minutes:

```bash
# Download the binary for your platform (example: Linux amd64)
curl -L -o muti-metroo https://download.mutimetroo.com/linux-amd64/muti-metroo
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Run interactive setup wizard
muti-metroo setup
```

Download binaries for all platforms from the [Download page](/download).

The wizard guides you through configuring your first agent, generating TLS certificates, and starting the mesh.

## Documentation Overview

| Section                                          | Description                                       |
| ------------------------------------------------ | ------------------------------------------------- |
| [Getting Started](/getting-started/overview)      | Installation, setup, and your first mesh          |
| [Core Concepts](/concepts/architecture)           | Architecture, roles, transports, and routing      |
| [Configuration](/configuration/overview)          | Complete configuration reference                  |
| [Features](/features/socks5-proxy)                | SOCKS5, exit routing, port forwarding, file transfer, shell |
| [Deployment](/deployment/scenarios)               | Docker, Kubernetes, and production deployment     |
| [Security](/security/overview)                    | TLS, authentication, and best practices           |
| [CLI Reference](/cli/overview)                    | Command-line interface documentation              |
| [HTTP API](/api/overview)                         | REST API for monitoring and management            |
| [Troubleshooting](/troubleshooting/common-issues) | Common issues and FAQ                             |

## Next Steps

- **New to Muti Metroo?** Start with [Getting Started](/getting-started/overview)
- **Want to understand the architecture?** Read [Core Concepts](/concepts/architecture)
- **Ready to deploy?** Check out [Deployment Scenarios](/deployment/scenarios)
- **Need help?** Visit [Troubleshooting](/troubleshooting/common-issues)
