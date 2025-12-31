---
slug: /
title: Introduction
sidebar_position: 1
---

# Muti Metroo

A userspace mesh networking agent written in Go that creates virtual TCP tunnels across heterogeneous transport layers. It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

## Key Features

- **Multiple Transport Layers**: QUIC/TLS 1.3, HTTP/2, and WebSocket transports
- **SOCKS5 Proxy Ingress**: Accept client connections with optional username/password authentication
- **CIDR-Based Exit Routing**: Advertise routes and handle DNS resolution at exit nodes
- **Multi-Hop Mesh Routing**: Flood-based route propagation with longest-prefix match
- **Stream Multiplexing**: Multiple virtual streams over a single peer connection with half-close support
- **File Transfer**: Upload and download files and directories to/from remote agents
- **Remote Procedure Call (RPC)**: Execute shell commands on remote agents for maintenance and diagnostics
- **Web Dashboard**: Embedded web interface with metro map visualization
- **Automatic Reconnection**: Exponential backoff with jitter for resilient peer connections
- **No Root Required**: Runs entirely in userspace

## Why Muti Metroo?

Muti Metroo solves the problem of creating resilient, multi-hop TCP tunnels across diverse network environments:

- **Heterogeneous Transports**: Mix QUIC, HTTP/2, and WebSocket in the same mesh to work through firewalls and restrictive networks
- **Multi-Hop Routing**: Route traffic through multiple intermediate nodes without configuring each hop manually
- **Dynamic Route Discovery**: Automatically propagate and discover routes through the mesh using flood-based routing
- **Flexible Agent Roles**: Each agent can simultaneously act as ingress (SOCKS5 proxy), transit (relay), and exit (external connections)

## Use Cases

- **Corporate Network Access**: Provide secure access to internal resources through multi-hop SOCKS5 proxy chains
- **Network Segmentation Bypass**: Route traffic through multiple network segments without VPN infrastructure
- **Resilient Remote Access**: Maintain connectivity through redundant paths and automatic reconnection
- **Development and Testing**: Create complex network topologies for testing distributed applications
- **File Transfer**: Securely transfer files to/from remote systems through the mesh
- **Remote Management**: Execute commands on remote agents for diagnostics and maintenance

## Quick Start

Get up and running in minutes:

```bash
# Download and build
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4
make build

# Run interactive setup wizard
./build/muti-metroo setup
```

The wizard will guide you through configuring your first agent, generating TLS certificates, and starting the mesh.

## Next Steps

- [Installation](installation) - Build from source or use Docker
- [Quick Start](quick-start) - Manual setup and configuration
- [Interactive Setup](interactive-setup) - Use the setup wizard
- [Configuration](configuration) - Complete configuration reference
- [Architecture](architecture/overview) - Understand how it works
