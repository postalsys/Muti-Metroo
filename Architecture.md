# Mesh Agent Network Architecture

**Version:** 2.3
**Date:** January 2026

## Executive Overview

Muti Metroo is a userspace mesh networking agent that creates encrypted virtual tunnels across heterogeneous transport layers (QUIC, HTTP/2, WebSocket). Agents form a mesh network where each can serve as ingress (SOCKS5 proxy entry point), transit (relay between networks), or exit (connection to target destinations). Traffic flows through multi-hop paths with automatic route discovery via flood-based propagation.

The architecture provides end-to-end encryption using X25519 key exchange and ChaCha20-Poly1305, ensuring transit agents cannot decrypt payload data. Stream multiplexing enables concurrent connections with full TCP semantics including half-close support. The system operates entirely in userspace without root privileges, making it suitable for deployment across diverse environments where traditional VPNs are impractical.

---

## Table of Contents

- [Executive Overview](#executive-overview)

1. [Executive Summary](#1-executive-summary)
2. [System Overview](#2-system-overview)
3. [Core Components](#3-core-components)
4. [Identity and Addressing](#4-identity-and-addressing)
5. [Transport Layer](#5-transport-layer)
6. [Frame Protocol](#6-frame-protocol)
   - [6.5 UDP Relay Protocol](#65-udp-relay-protocol)
   - [6.6 Port Forwarding](#66-port-forwarding-reverse-tunnel)
   - [6.7 ICMP Echo Protocol](#67-icmp-echo-protocol)
   - [6.8 Sleep Mode Protocol](#68-sleep-mode-protocol)
7. [Stream Management](#7-stream-management)
8. [Routing System](#8-routing-system)
9. [Flood Protocol](#9-flood-protocol)
10. [Peer Connection Management](#10-peer-connection-management)
11. [SOCKS5 Server](#11-socks5-server)
12. [Data Plane](#12-data-plane)
13. [Configuration](#13-configuration)
14. [Security](#14-security)
15. [Observability](#15-observability)
16. [Operations](#16-operations)
17. [Certificate Management](#17-certificate-management)
18. [Project Structure](#18-project-structure)
19. [Implementation Notes](#19-implementation-notes)
20. [Testing Strategy](#20-testing-strategy)

- [Appendix A: Quick Reference](#appendix-a-quick-reference)
- [Appendix B: Mutiauk - Optional TUN Interface](#appendix-b-mutiauk---optional-tun-interface)

---

## 1. Executive Summary

### 1.1 Purpose

This document describes the architecture for a userspace mesh networking agent that creates virtual TCP tunnels across heterogeneous transport layers. The system enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

### 1.2 Design Goals

| Goal                      | Description                                                 |
| ------------------------- | ----------------------------------------------------------- |
| **Userspace operation**   | No kernel modules, no root/admin privileges required        |
| **Transport flexibility** | Support QUIC, HTTP/2, and WebSocket to traverse any network |
| **Multi-hop routing**     | Chain agents across network boundaries                      |
| **Bidirectional streams** | Full TCP semantics including half-close                     |
| **Automatic recovery**    | Reconnect on failure, re-advertise routes                   |
| **Low latency**           | Suitable for interactive applications (SSH)                 |
| **High throughput**       | Capable of streaming video                                  |
| **Production ready**      | Health checks, service management                           |

### 1.3 Target Scale

This architecture targets small to medium deployments:

- **Agents:** Up to 20
- **Concurrent users:** Up to 10
- **Traffic types:** Interactive (SSH), streaming (video), bulk transfers
- **Typical topology:** Chains of 2-5 agents with occasional branching

The design prioritizes simplicity and correctness over extreme scalability.

### 1.4 Implementation Language

The reference implementation is written in **Go**, leveraging its excellent concurrency primitives and networking libraries.

---

## 2. System Overview

### 2.1 Network Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MESH NETWORK                                   │
│                                                                             │
│  ┌─────────┐      QUIC       ┌─────────┐     HTTP/2     ┌─────────┐         │
│  │ Agent 1 │◄───────────────►│ Agent 2 │◄──────────────►│ Agent 3 │         │
│  │         │    (UDP/4433)   │         │   (TCP/8443)   │         │         │
│  │ Entry   │                 │ Transit │                │  Exit   │         │
│  │ SOCKS5  │                 │         │                │ 10.0.0.0/8        │
│  │ :1080   │                 │         │                │         │         │
│  └────▲────┘                 └─────────┘                └────┬────┘         │
│       │                                                      │              │
│       │ Client                                          Real │ TCP          │
│       │ Application                                          │              │
│  ┌────┴────┐                                           ┌─────▼─────┐        │
│  │  curl   │                                           │  Target   │        │
│  │ Browser │                                           │  Server   │        │
│  │  SSH    │                                           │ 10.5.3.100│        │
│  └─────────┘                                           └───────────┘        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Agent Roles

An agent can serve one or more roles simultaneously:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AGENT ROLES                                    │
│                                                                             │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐        │
│  │      INGRESS      │  │      TRANSIT      │  │       EXIT        │        │
│  │                   │  │                   │  │                   │        │
│  │ • SOCKS5 listener │  │ • Forward streams │  │ • Open real TCP   │        │
│  │ • Initiates       │  │ • Route flooding  │  │ • Advertise CIDRs │        │
│  │   virtual streams │  │ • No local I/O    │  │ • DNS resolution  │        │
│  │ • Route lookup    │  │                   │  │                   │        │
│  └───────────────────┘  └───────────────────┘  └───────────────────┘        │
│                                                                             │
│  Deployment Examples:                                                       │
│  • Home laptop:     Ingress only (SOCKS5 for browser)                       │
│  • Cloud relay:     Transit only (forward between networks)                 │
│  • Office gateway:  Exit only (access to internal network)                  │
│  • VPN endpoint:    Ingress + Exit (full VPN replacement)                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Traffic Flow Example

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         END-TO-END TRAFFIC FLOW                             │
│                                                                             │
│  1. User runs: ssh -o ProxyCommand='nc -x localhost:1080 %h %p' server      │
│                                                                             │
│  2. SSH connects to SOCKS5 proxy on Agent1 (localhost:1080)                 │
│                                                                             │
│  3. SOCKS5 receives: CONNECT server.internal:22                             │
│                                                                             │
│  4. Agent1 looks up route for "server.internal"                             │
│     Result: Exit=Agent3, Path=[Agent2, Agent3]                              │
│                                                                             │
│  5. Agent1 sends STREAM_OPEN to Agent2:                                     │
│     - Destination: server.internal:22                                       │
│     - RemainingPath: [Agent3]                                               │
│                                                                             │
│  6. Agent2 forwards STREAM_OPEN to Agent3:                                  │
│     - RemainingPath: [] (empty = I am exit)                                 │
│                                                                             │
│  7. Agent3 resolves "server.internal" via local DNS                         │
│     Opens TCP connection to server.internal:22                              │
│     Sends STREAM_OPEN_ACK back through chain                                │
│                                                                             │
│  8. Agent1 receives ACK, sends SOCKS5 success to SSH                        │
│                                                                             │
│  9. SSH session data flows bidirectionally through the chain                │
│     User ↔ Agent1 ↔ Agent2 ↔ Agent3 ↔ SSH Server                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Components

### 3.1 Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AGENT PROCESS                                  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                          CONTROL PLANE                                │  │
│  │                                                                       │  │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │  │
│  │  │  Route Table   │  │ Flood Protocol │  │  Peer Manager  │           │  │
│  │  │                │  │                │  │                │           │  │
│  │  │ • CIDR entries │  │ • Advertise    │  │ • Connections  │           │  │
│  │  │ • LPM lookup   │  │ • Withdraw     │  │ • Reconnection │           │  │
│  │  │ • Path cache   │  │ • Loop prevent │  │ • Handshake    │           │  │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                           DATA PLANE                                  │  │
│  │                                                                       │  │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │  │
│  │  │ Stream Manager │  │ Forward Table  │  │  Exit Handler  │           │  │
│  │  │                │  │                │  │                │           │  │
│  │  │ • Lifecycle    │  │ • Stream map   │  │ • TCP connect  │           │  │
│  │  │ • Half-close   │  │ • Relay data   │  │ • DNS resolve  │           │  │
│  │  │ • Fairness     │  │ • Cleanup      │  │ • Error handle │           │  │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        TRANSPORT LAYER                                │  │
│  │                                                                       │  │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │  │
│  │  │     QUIC       │  │    HTTP/2      │  │   WebSocket    │           │  │
│  │  │    (UDP)       │  │   Streaming    │  │   HTTP/1.1     │           │  │
│  │  │                │  │                │  │                │           │  │
│  │  │ • Native mux   │  │ • Single POST  │  │ • Proxy compat │           │  │
│  │  │ • 0-RTT        │  │ • Bidi stream  │  │ • Upgrade      │           │  │
│  │  │ • Migration    │  │ • Port 443     │  │ • Binary frame │           │  │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         INGRESS LAYER                                 │  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │                       SOCKS5 Server                             │  │  │
│  │  │  • CONNECT command  • IPv4/IPv6/Domain  • Optional auth         │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                       OBSERVABILITY LAYER                             │  │
│  │                                                                       │  │
│  │  ┌──────────────────────────────┐  ┌────────────────┐                 │  │
│  │  │         Health Check         │  │   Control API  │                 │  │
│  │  │           (HTTP)             │  │ (Unix Socket)  │                 │  │
│  │  └──────────────────────────────┘  └────────────────┘                 │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Component Responsibilities

| Component               | Responsibility                                          |
| ----------------------- | ------------------------------------------------------- |
| **Route Table**         | Store CIDR→Path mappings, perform longest-prefix match  |
| **Flood Protocol**      | Propagate route advertisements between peers            |
| **Peer Manager**        | Manage peer connections, handle reconnection            |
| **Stream Manager**      | Track virtual stream lifecycle and state                |
| **Forward Table**       | Map incoming streams to outgoing destinations           |
| **Exit Handler**        | Open real TCP connections, handle DNS                   |
| **QUIC Transport**      | High-performance UDP transport with native multiplexing |
| **HTTP/2 Transport**    | TCP streaming for direct connections                    |
| **WebSocket Transport** | HTTP/1.1 WebSocket for proxy traversal                  |
| **SOCKS5 Server**       | Accept client connections, initiate streams             |
| **Health Check**        | HTTP endpoints for liveness/readiness probes            |
| **Control API**         | Unix socket API for management commands                 |

---

## 4. Identity and Addressing

### 4.1 Agent Identity

Each agent has a persistent unique identifier:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             AGENT IDENTITY                                  │
│                                                                             │
│  AgentID: 16 bytes (128 bits)                                               │
│                                                                             │
│  • Generated randomly on first run using crypto/rand                        │
│  • Displayed as hex string: "a3f8c2d1e5b94a7c8d2e1f0a3b5c7d9e"              │
│  • Short form for logs: "a3f8c2d1" (first 8 hex chars)                      │
│                                                                             │
│  Identity Storage Options:                                                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  FILE-BASED (default):                                                │  │
│  │    • AgentID:    {data_dir}/agent_id                                  │  │
│  │    • PrivateKey: {data_dir}/agent_key       (0600 permissions)        │  │
│  │    • PublicKey:  {data_dir}/agent_key.pub   (0644 permissions)        │  │
│  │                                                                       │  │
│  │  CONFIG-BASED (for single-file deployment):                           │  │
│  │    • agent.id:          32-char hex string                            │  │
│  │    • agent.private_key: 64-char hex string (X25519 private key)       │  │
│  │    • agent.public_key:  optional (derived from private_key)           │  │
│  │                                                                       │  │
│  │  When config-based identity is set:                                   │  │
│  │    • Takes precedence over file-based identity                        │  │
│  │    • data_dir becomes optional                                        │  │
│  │    • Enables true single-file deployment (no external files)          │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4.1.1 X25519 Keypair for E2E Encryption

Each agent has an X25519 keypair used for end-to-end encryption key exchange:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          E2E ENCRYPTION KEYPAIR                             │
│                                                                             │
│  Type:    X25519 (Curve25519 ECDH)                                          │
│  Size:    32 bytes (256 bits) each                                          │
│                                                                             │
│  Storage precedence:                                                        │
│    1. Config (agent.private_key) - if set, used exclusively                 │
│    2. Files ({data_dir}/agent_key) - default, auto-generated                │
│                                                                             │
│  Config-based identity validation rules:                                    │
│    • private_key alone is valid (public_key derived automatically)          │
│    • public_key without private_key is invalid                              │
│    • If public_key provided, must match derivation from private_key         │
│    • data_dir required only when private_key not in config                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Stream Identification

Streams are identified by a combination of peer connection and stream ID:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          STREAM IDENTIFICATION                              │
│                                                                             │
│  Stream IDs are scoped to a peer connection.                                │
│                                                                             │
│  Allocation scheme (prevents collisions):                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Connection initiator (dialer):   ODD  stream IDs (1, 3, 5, 7...)   │    │
│  │  Connection acceptor (listener):  EVEN stream IDs (2, 4, 6, 8...)   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Each side maintains its own counter, incrementing by 2.                    │
│  StreamID 0 is reserved for the control channel.                            │
│                                                                             │
│  Global stream reference (for logging/debugging):                           │
│    {PeerID}:{StreamID}                                                      │
│    Example: "a3f8c2d1:42"                                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.3 Address Formats

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            ADDRESS FORMATS                                  │
│                                                                             │
│  Transport addresses (configuration):                                       │
│                                                                             │
│    QUIC:       quic://192.168.1.50:4433                                     │
│                quic://[2001:db8::1]:4433                                    │
│                                                                             │
│    HTTP/2:     h2://192.168.1.50:8443/mesh                                  │
│                h2://relay.example.com:443/mesh                              │
│                                                                             │
│    WebSocket:  ws://192.168.1.50:8080/mesh                                  │
│                wss://relay.example.com:443/mesh                             │
│                                                                             │
│  Destination addresses (STREAM_OPEN):                                       │
│                                                                             │
│    Type 0x01:  IPv4      4 bytes     (e.g., 10.5.3.100)                     │
│    Type 0x04:  IPv6      16 bytes    (e.g., 2001:db8::1)                    │
│    Type 0x03:  Domain    1+N bytes   (length-prefixed string)               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. Transport Layer

### 5.1 Transport Selection Guide

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TRANSPORT SELECTION GUIDE                            │
│                                                                             │
│                           ┌─────────────┐                                   │
│                           │   Start     │                                   │
│                           └──────┬──────┘                                   │
│                                  │                                          │
│                                  ▼                                          │
│                        ┌─────────────────┐                                  │
│                        │ UDP permitted?  │                                  │
│                        └────────┬────────┘                                  │
│                                 │                                           │
│                    YES          │          NO                               │
│              ┌──────────────────┴──────────────────┐                        │
│              │                                     │                        │
│              ▼                                     ▼                        │
│     ┌─────────────────┐                  ┌─────────────────┐                │
│     │      QUIC       │                  │ Direct TCP to   │                │
│     │                 │                  │ peer possible?  │                │
│     │ • Best latency  │                  └────────┬────────┘                │
│     │ • Native mux    │                           │                         │
│     │ • 0-RTT resume  │              YES          │          NO             │
│     └─────────────────┘         ┌─────────────────┴─────────────────┐       │
│                                 │                                   │       │
│                                 ▼                                   ▼       │
│                        ┌─────────────────┐                ┌─────────────────┐
│                        │     HTTP/2      │                │   WebSocket     │
│                        │                 │                │                 │
│                        │ • Single TCP    │                │ • Proxy compat  │
│                        │ • Port 443 OK   │                │ • HTTP CONNECT  │
│                        │ • Good perf     │                │ • Most flexible │
│                        └─────────────────┘                └─────────────────┘
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Transport Comparison

| Aspect                    | QUIC                 | HTTP/2            | WebSocket            |
| ------------------------- | -------------------- | ----------------- | -------------------- |
| **Underlying**            | UDP                  | TCP               | TCP                  |
| **Multiplexing**          | Native               | Application-layer | Application-layer    |
| **Head-of-line blocking** | None                 | TCP level         | TCP level            |
| **Connection setup**      | 1-RTT (0-RTT resume) | TCP + TLS         | TCP + TLS + Upgrade  |
| **Proxy traversal**       | Poor                 | Moderate          | Excellent            |
| **Firewall friendliness** | Poor                 | Good              | Excellent            |
| **Best for**              | Performance          | Direct TCP        | Restrictive networks |

### 5.3 Transport Interface

All transports implement a unified interface:

```go
// Transport creates and accepts peer connections
type Transport interface {
    // Dial connects to a remote peer
    Dial(ctx context.Context, addr string, opts DialOptions) (PeerConn, error)

    // Listen creates a listener for incoming connections
    Listen(addr string, opts ListenOptions) (Listener, error)

    // Type returns the transport type identifier
    Type() TransportType

    // Close closes the transport
    Close() error
}

// Listener accepts incoming peer connections
type Listener interface {
    // Accept waits for and returns the next connection
    Accept(ctx context.Context) (PeerConn, error)

    // Addr returns the listener's network address
    Addr() net.Addr

    // Close stops the listener
    Close() error
}

// PeerConn represents a connection to a peer
type PeerConn interface {
    // OpenStream creates a new outgoing stream
    OpenStream(ctx context.Context) (Stream, error)

    // AcceptStream waits for an incoming stream
    AcceptStream(ctx context.Context) (Stream, error)

    // Close terminates the connection
    Close() error

    // LocalAddr returns the local address
    LocalAddr() net.Addr

    // RemoteAddr returns the remote address
    RemoteAddr() net.Addr
}

// Stream is a bidirectional byte stream
type Stream interface {
    io.Reader
    io.Writer

    // CloseWrite sends a half-close (FIN)
    CloseWrite() error

    // Close fully closes the stream
    Close() error

    // SetDeadline sets read and write deadlines
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error

    // StreamID returns the stream identifier
    StreamID() uint64
}
```

### 5.4 QUIC Transport

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            QUIC TRANSPORT                                   │
│                                                                             │
│  ┌───────────────┐            UDP            ┌───────────────┐              │
│  │    Agent A    │◄─────────────────────────►│    Agent B    │              │
│  │               │      QUIC Connection      │               │              │
│  │  ┌─────────┐  │                           │  ┌─────────┐  │              │
│  │  │Stream 0 │◄─┼───────────────────────────┼─►│Stream 0 │  │  Control     │
│  │  ├─────────┤  │                           │  ├─────────┤  │              │
│  │  │Stream 1 │◄─┼───────────────────────────┼─►│Stream 1 │  │  Data        │
│  │  ├─────────┤  │                           │  ├─────────┤  │              │
│  │  │Stream 3 │◄─┼───────────────────────────┼─►│Stream 3 │  │  Data        │
│  │  ├─────────┤  │                           │  ├─────────┤  │              │
│  │  │   ...   │  │                           │  │   ...   │  │              │
│  │  └─────────┘  │                           │  └─────────┘  │              │
│  └───────────────┘                           └───────────────┘              │
│                                                                             │
│  Implementation: github.com/quic-go/quic-go                                 │
│                                                                             │
│  Characteristics:                                                           │
│  • Each virtual stream maps to native QUIC stream                           │
│  • No head-of-line blocking between streams                                 │
│  • Built-in TLS 1.3 encryption                                              │
│  • Connection migration (survives IP changes)                               │
│  • 0-RTT session resumption                                                 │
│                                                                             │
│  Configuration:                                                             │
│  • Max streams: 10,000                                                      │
│  • Idle timeout: 60s                                                        │
│  • Keepalive: 30s                                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.5 HTTP/2 Streaming Transport

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        HTTP/2 STREAMING TRANSPORT                           │
│                                                                             │
│  ┌───────────────┐          TCP+TLS          ┌───────────────┐              │
│  │    Agent A    │◄─────────────────────────►│    Agent B    │              │
│  │   (client)    │       HTTP/2 Conn         │   (server)    │              │
│  │               │                           │               │              │
│  │               │     POST /mesh            │               │              │
│  │  ┌─────────┐  │     ─────────────►        │  ┌─────────┐  │              │
│  │  │ Request │  │     Streaming body        │  │ Handler │  │              │
│  │  │  body   │──┼──────────────────────────►│──│  reads  │  │              │
│  │  │ writer  │  │                           │  │ request │  │              │
│  │  └─────────┘  │                           │  └─────────┘  │              │
│  │               │     ◄─────────────        │               │              │
│  │  ┌─────────┐  │     Streaming resp        │  ┌─────────┐  │              │
│  │  │Response │◄─┼───────────────────────────┼──│Response │  │              │
│  │  │  body   │  │                           │  │ writer  │  │              │
│  │  │ reader  │  │                           │  └─────────┘  │              │
│  │  └─────────┘  │                           │               │              │
│  └───────────────┘                           └───────────────┘              │
│                                                                             │
│  Implementation: golang.org/x/net/http2                                     │
│                                                                             │
│  Characteristics:                                                           │
│  • Single HTTP/2 POST request with streaming body                           │
│  • Response body also streams (bidirectional once established)              │
│  • Our frame protocol provides multiplexing                                 │
│  • Works on port 443, traverses most firewalls                              │
│                                                                             │
│  Key implementation details:                                                │
│  • Use io.Pipe for request body streaming                                   │
│  • Call http.Flusher.Flush() to disable buffering                           │
│  • Set Content-Type: application/octet-stream                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.6 WebSocket HTTP/1.1 Transport

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      WEBSOCKET HTTP/1.1 TRANSPORT                           │
│                                                                             │
│  ┌───────────────┐    ┌───────────┐    ┌───────────────┐                    │
│  │    Agent A    │────│   HTTP    │────│    Agent B    │                    │
│  │   (client)    │    │   Proxy   │    │   (server)    │                    │
│  └───────┬───────┘    └─────┬─────┘    └───────┬───────┘                    │
│          │                  │                  │                            │
│          │ CONNECT host:443 │                  │                            │
│          │─────────────────►│                  │                            │
│          │                  │ TCP connect      │                            │
│          │                  │─────────────────►│                            │
│          │◄─────────────────│                  │                            │
│          │ 200 Tunnel OK    │                  │                            │
│          │                                     │                            │
│          │◄════════════ TLS Handshake ════════►│                            │
│          │                                     │                            │
│          │ GET /mesh HTTP/1.1                  │                            │
│          │ Upgrade: websocket                  │                            │
│          │────────────────────────────────────►│                            │
│          │                                     │                            │
│          │◄────────────────────────────────────│                            │
│          │ 101 Switching Protocols             │                            │
│          │                                     │                            │
│          │◄═══════ WebSocket Frames ══════════►│                            │
│          │         (bidirectional)             │                            │
│                                                                             │
│  Implementation: nhooyr.io/websocket                                        │
│                                                                             │
│  Characteristics:                                                           │
│  • Maximum compatibility with corporate proxies                             │
│  • HTTP CONNECT tunneling for proxy traversal                               │
│  • After upgrade, fully symmetric bidirectional                             │
│  • Binary frames for efficiency                                             │
│  • Our frame protocol provides multiplexing                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.7 Multiplexing Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MULTIPLEXING STRATEGY                               │
│                                                                             │
│  QUIC:                                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Each virtual stream = dedicated QUIC stream                        │    │
│  │  No additional framing needed for multiplexing                      │    │
│  │  Frames still used for control messages and stream metadata         │    │
│  │  Benefit: Native per-stream flow control, no HOL blocking           │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  HTTP/2 and WebSocket:                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Single transport stream carries all virtual streams                │    │
│  │  Our frame protocol provides multiplexing via StreamID              │    │
│  │  Writer uses round-robin for fairness between streams               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Fairness (HTTP/2 and WebSocket):                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Problem: Video stream could starve SSH stream                      │    │
│  │                                                                     │    │
│  │  Solution:                                                          │    │
│  │  • Maximum frame payload: 16 KB                                     │    │
│  │  • Writer maintains queue per stream                                │    │
│  │  • Round-robin between streams with pending data                    │    │
│  │  • No stream can send more than one frame before others get a turn  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Frame Protocol

### 6.1 Frame Format

All communication uses a consistent framing protocol:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             FRAME FORMAT                                    │
│                                                                             │
│    0       1       2       3       4       5       6       7       8        │
│   ┌───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┐         │
│   │ Type  │ Flags │            Length             │    StreamID   │         │
│   │  1B   │  1B   │              4B               │       8B      │         │
│   ├───────┴───────┴───────────────────────────────┴───────────────┤         │
│   │                                                               │         │
│   │                          Payload                              │         │
│   │                       (Length bytes)                          │         │
│   │                                                               │         │
│   └───────────────────────────────────────────────────────────────┘         │
│                                                                             │
│   Header size: 14 bytes                                                     │
│   Maximum payload: 16,384 bytes (16 KB)                                     │
│   Maximum frame size: 16,398 bytes                                          │
│                                                                             │
│   Byte order: Big-endian (network byte order)                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Frame Types

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             FRAME TYPES                                     │
│                                                                             │
│  Stream Frames:                                                             │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐  │
│  │ Type │ Name               │ Direction   │ Purpose                     │  │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤  │
│  │ 0x01 │ STREAM_OPEN        │ Forward     │ Request to open stream      │  │
│  │ 0x02 │ STREAM_OPEN_ACK    │ Backward    │ Stream opened successfully  │  │
│  │ 0x03 │ STREAM_OPEN_ERR    │ Backward    │ Stream open failed          │  │
│  │ 0x04 │ STREAM_DATA        │ Both        │ Payload data                │  │
│  │ 0x05 │ STREAM_CLOSE       │ Both        │ Graceful close (half/full)  │  │
│  │ 0x06 │ STREAM_RESET       │ Both        │ Abort stream with error     │  │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘  │
│                                                                             │
│  Routing Frames:                                                            │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐  │
│  │ Type │ Name               │ Direction   │ Purpose                     │  │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤  │
│  │ 0x10 │ ROUTE_ADVERTISE    │ Flood       │ Announce CIDR/domain routes │  │
│  │ 0x11 │ ROUTE_WITHDRAW     │ Flood       │ Remove CIDR routes          │  │
│  │ 0x12 │ NODE_INFO_ADVERTISE│ Flood       │ Announce node metadata      │  │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘  │
│                                                                             │
│  Route Address Families:                                                    │
│  ┌──────┬──────────────────┬────────────────────────────────────────────┐   │
│  │ Type │ Name             │ Description                                │   │
│  ├──────┼──────────────────┼────────────────────────────────────────────┤   │
│  │ 0x01 │ AddrFamilyIPv4   │ IPv4 CIDR route (e.g., 10.0.0.0/8)         │   │
│  │ 0x02 │ AddrFamilyIPv6   │ IPv6 CIDR route (e.g., ::/0)               │   │
│  │ 0x03 │ AddrFamilyDomain │ Domain pattern (exact or wildcard)         │   │
│  │ 0x04 │ AddrFamilyForward│ Port forward routing key                   │   │
│  │ 0x05 │ AddrFamilyAgent  │ Agent presence (128-bit agent ID)           │   │
│  └──────┴──────────────────┴────────────────────────────────────────────┘   │
│                                                                             │
│  Agent Presence Routes:                                                    │
│  Every agent unconditionally advertises a presence route (AddrFamilyAgent) │
│  containing its 128-bit agent ID. This makes all agents reachable by ID   │
│  for control requests, shell, file transfer, and ICMP -- even without     │
│  exit routes configured. Presence routes use metric 0 and are advertised  │
│  alongside any CIDR/domain/forward routes.                                │
│                                                                             │
│  Domain Route Wire Format in ROUTE_ADVERTISE:                               │
│  • AddressFamily: 0x03 (domain)                                             │
│  • PrefixLength: 0x00 (exact) or 0x01 (wildcard)                            │
│  • Prefix: 1-byte length + UTF-8 domain pattern                             │
│  • Metric: uint16                                                           │
│                                                                             │
│  Control Frames:                                                            │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐  │
│  │ Type │ Name               │ Direction   │ Purpose                     │  │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤  │
│  │ 0x20 │ PEER_HELLO         │ Initiator   │ Initial handshake           │  │
│  │ 0x21 │ PEER_HELLO_ACK     │ Acceptor    │ Handshake response          │  │
│  │ 0x22 │ KEEPALIVE          │ Either      │ Liveness probe              │  │
│  │ 0x23 │ KEEPALIVE_ACK      │ Either      │ Liveness response           │  │
│  │ 0x24 │ CONTROL_REQUEST    │ Either      │ Request status/RPC          │  │
│  │ 0x25 │ CONTROL_RESPONSE   │ Either      │ Response with data          │  │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘  │
│                                                                             │
│  Control Request Types (in CONTROL_REQUEST payload):                        │
│  ┌──────┬────────────────────┬──────────────────────────────────────────┐   │
│  │ Type │ Name               │ Purpose                                  │   │
│  ├──────┼────────────────────┼──────────────────────────────────────────┤   │
│  │ 0x02 │ STATUS             │ Request agent status                     │   │
│  │ 0x03 │ PEERS              │ Request peer list                        │   │
│  │ 0x04 │ ROUTES             │ Request route table                      │   │
│  │ 0x05 │ RPC                │ Remote procedure call (shell command)    │   │
│  │ 0x08 │ ROUTE_MANAGE       │ Add, remove, or list dynamic routes      │   │
│  └──────┴────────────────────┴──────────────────────────────────────────┘   │
│                                                                             │
│  UDP Frames (for SOCKS5 UDP ASSOCIATE):                                     │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐  │
│  │ Type │ Name               │ Direction   │ Purpose                     │  │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤  │
│  │ 0x30 │ UDP_OPEN           │ Forward     │ Request UDP association     │  │
│  │ 0x31 │ UDP_OPEN_ACK       │ Backward    │ Association established     │  │
│  │ 0x32 │ UDP_OPEN_ERR       │ Backward    │ Association failed          │  │
│  │ 0x33 │ UDP_DATAGRAM       │ Both        │ UDP datagram payload        │  │
│  │ 0x34 │ UDP_CLOSE          │ Both        │ Close association           │  │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘  │
│                                                                             │
│  File Transfer: Uses special domain addresses in STREAM_OPEN:               │
│  • "file:upload" - Upload file to remote agent                              │
│  • "file:download" - Download file from remote agent                        │
│                                                                             │
│  ICMP Frames (for ping through mesh):                                       │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐  │
│  │ Type │ Name               │ Direction   │ Purpose                     │  │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤  │
│  │ 0x40 │ ICMP_OPEN          │ Forward     │ Request ICMP echo session   │  │
│  │ 0x41 │ ICMP_OPEN_ACK      │ Backward    │ Session established         │  │
│  │ 0x42 │ ICMP_OPEN_ERR      │ Backward    │ Session failed              │  │
│  │ 0x43 │ ICMP_ECHO          │ Both        │ Echo request/reply payload  │  │
│  │ 0x44 │ ICMP_CLOSE         │ Both        │ Close ICMP session          │  │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.3 Frame Flags

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              FRAME FLAGS                                    │
│                                                                             │
│   Bit   │ Name          │ Applicable To          │ Meaning                  │
│  ───────┼───────────────┼────────────────────────┼───────────────────────── │
│    0    │ FIN_WRITE     │ STREAM_DATA/CLOSE      │ Sender is done writing   │
│    1    │ FIN_READ      │ STREAM_CLOSE           │ Sender is done reading   │
│    2    │ (reserved)    │                        │                          │
│    3    │ (reserved)    │                        │                          │
│   4-7   │ (reserved)    │                        │                          │
│                                                                             │
│   FIN_WRITE can be set on STREAM_DATA to signal half-close with final data  │
│   or on STREAM_CLOSE for half-close without additional data.                │
│                                                                             │
│   Flag combinations on STREAM_CLOSE:                                        │
│   0x01 = FIN_WRITE only    -> Half-close (done sending)                     │
│   0x02 = FIN_READ only     -> Half-close (done receiving)                   │
│   0x03 = FIN_WRITE|READ    -> Full close                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.4 Payload Definitions

#### PEER_HELLO (0x20)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PEER_HELLO                                     │
│                                                                             │
│   Sent by connection initiator after transport is established.              │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐   │
│   │ Field           │ Size   │ Description                              │   │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤   │
│   │ Version         │ 2      │ Protocol version (currently 1)           │   │
│   │ AgentID         │ 16     │ Sender's agent ID                        │   │
│   │ Timestamp       │ 8      │ Unix timestamp (seconds)                 │   │
│   │ CapabilitiesLen │ 1      │ Number of capabilities                   │   │
│   │ Capabilities    │ varies │ List of capability strings               │   │
│   └─────────────────┴────────┴──────────────────────────────────────────┘   │
│                                                                             │
│   Capability string format: 1-byte length + UTF-8 string                    │
│   Known capabilities: "exit", "socks5"                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN (0x01)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             STREAM_OPEN                                     │
│                                                                             │
│   Request to open a virtual stream to a destination.                        │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐   │
│   │ Field           │ Size   │ Description                              │   │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤   │
│   │ RequestID       │ 8      │ Unique ID for correlating ACK/ERR        │   │
│   │ AddressType     │ 1      │ 0x01=IPv4, 0x04=IPv6, 0x03=Domain        │   │
│   │ Address         │ varies │ Address bytes (see below)                │   │
│   │ Port            │ 2      │ Destination port                         │   │
│   │ TTL             │ 1      │ Remaining hops (decremented each hop)    │   │
│   │ PathLength      │ 1      │ Number of agents in remaining path       │   │
│   │ RemainingPath   │ varies │ Array of AgentIDs (16 bytes each)        │   │
│   │ EphemeralPubKey │ 32     │ X25519 public key for E2E encryption     │   │
│   └─────────────────┴────────┴──────────────────────────────────────────┘   │
│                                                                             │
│   Address encoding:                                                         │
│   • IPv4 (0x01):   4 bytes, network order                                   │
│   • IPv6 (0x04):   16 bytes, network order                                  │
│   • Domain (0x03): 1-byte length + UTF-8 domain name                        │
│                                                                             │
│   The ephemeral public key is used to establish E2E encryption between      │
│   ingress and exit agents. Transit agents forward this key unchanged.       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN_ACK (0x02)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STREAM_OPEN_ACK                                   │
│                                                                             │
│   Sent by exit agent to acknowledge successful stream open.                 │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐   │
│   │ Field           │ Size   │ Description                              │   │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤   │
│   │ RequestID       │ 8      │ Correlates with STREAM_OPEN request      │   │
│   │ BoundAddrType   │ 1      │ 0x01=IPv4, 0x04=IPv6                     │   │
│   │ BoundAddress    │ 4 or 16│ Bound local address                      │   │
│   │ BoundPort       │ 2      │ Bound local port                         │   │
│   │ EphemeralPubKey │ 32     │ Exit's X25519 public key for E2E         │   │
│   └─────────────────┴────────┴──────────────────────────────────────────┘   │
│                                                                             │
│   The ephemeral public key allows the ingress agent to compute the same     │
│   shared secret via X25519 ECDH for end-to-end encryption.                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_DATA (0x04)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             STREAM_DATA                                     │
│                                                                             │
│   Encrypted stream data payload. Max payload: 16 KB.                        │
│                                                                             │
│   Payload is encrypted with ChaCha20-Poly1305:                              │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐   │
│   │ Field           │ Size   │ Description                              │   │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤   │
│   │ Nonce           │ 12     │ Counter (8 bytes) + direction bit        │   │
│   │ Ciphertext      │ varies │ Encrypted application data               │   │
│   │ AuthTag         │ 16     │ Poly1305 authentication tag              │   │
│   └─────────────────┴────────┴──────────────────────────────────────────┘   │
│                                                                             │
│   Encryption overhead: 28 bytes per frame                                   │
│                                                                             │
│   Flags:                                                                    │
│   • FIN_WRITE (0x01): Sender half-close (no more writes)                    │
│   • FIN_READ (0x02): Receiver half-close (no more reads)                    │
│                                                                             │
│   Transit agents forward encrypted payloads unchanged (cannot decrypt).     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN_ERR (0x03) Error Codes

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ERROR CODES                                       │
│                                                                             │
│   ┌───────┬──────────────────────┬────────────────────────────────────┐     │
│   │ Code  │ Name                 │ Meaning                            │     │
│   ├───────┼──────────────────────┼────────────────────────────────────┤     │
│   │ 1     │ NO_ROUTE             │ No route to destination            │     │
│   │ 2     │ CONNECTION_REFUSED   │ Target refused connection          │     │
│   │ 3     │ CONNECTION_TIMEOUT   │ Connection attempt timed out       │     │
│   │ 4     │ TTL_EXCEEDED         │ TTL reached zero                   │     │
│   │ 5     │ HOST_UNREACHABLE     │ Cannot reach target host           │     │
│   │ 6     │ NETWORK_UNREACHABLE  │ Cannot reach target network        │     │
│   │ 7     │ DNS_ERROR            │ Domain name resolution failed      │     │
│   │ 8     │ EXIT_DISABLED        │ Exit functionality not enabled     │     │
│   │ 9     │ RESOURCE_LIMIT       │ Too many streams                   │     │
│   │ 10    │ CONNECTION_LIMIT     │ Connection limit exceeded          │     │
│   │ 11    │ NOT_ALLOWED          │ Operation not permitted            │     │
│   │ 12    │ FILE_TRANSFER_DENIED │ File transfer not allowed          │     │
│   │ 13    │ AUTH_REQUIRED        │ Authentication required            │     │
│   │ 14    │ PATH_NOT_ALLOWED     │ Path not in allowed list           │     │
│   │ 15    │ FILE_TOO_LARGE       │ File exceeds size limit            │     │
│   │ 16    │ FILE_NOT_FOUND       │ File does not exist                │     │
│   │ 17    │ WRITE_FAILED         │ Write operation failed             │     │
│   │ 18    │ GENERAL_FAILURE      │ General error (e.g., key exchange) │     │
│   │ 30    │ UDP_DISABLED         │ UDP relay is disabled              │     │
│   │ 50    │ ICMP_DISABLED        │ ICMP feature is disabled           │     │
│   │ 51    │ ICMP_DEST_NOT_ALLOWED│ Destination not in allowed CIDRs   │     │
│   │ 52    │ ICMP_SESSION_LIMIT   │ Max concurrent sessions reached    │     │
│   └───────┴──────────────────────┴────────────────────────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### NODE_INFO_ADVERTISE (0x12)

Flooded through the mesh to announce node metadata. The NodeInfo payload may be encrypted
with the management key for topology compartmentalization.

**Envelope (NodeInfoAdvertise):**

```
+------------------+--------+--------------------------------------------------+
| Field            | Size   | Description                                      |
+------------------+--------+--------------------------------------------------+
| OriginAgent      | 16     | Agent advertising its info (128-bit AgentID)     |
| Sequence         | 8      | Monotonically increasing sequence (uint64)       |
| Encrypted        | 1      | 0x00 = plaintext, 0x01 = encrypted               |
| DataLen          | 2      | Length of NodeInfo data (uint16)                  |
| Data             | varies | NodeInfo bytes (plaintext or encrypted blob)     |
| SeenByCount      | 1      | Number of agents in SeenBy list                  |
| SeenBy[]         | N*16   | Agent IDs for loop prevention                    |
+------------------+--------+--------------------------------------------------+
```

When `Encrypted` is 0x01, the `Data` field contains `EphemeralPub(32) + Nonce(24) + Ciphertext + Tag(16)`
and must be decrypted with the management key before decoding as NodeInfo.

**NodeInfo payload (encoding order):**

```
+---------------------------+--------+--------------------------------------------------+
| Field                     | Size   | Description                                      |
+---------------------------+--------+--------------------------------------------------+
| DisplayName               | 1+N    | Length-prefixed UTF-8 string                     |
| Hostname                  | 1+N    | Length-prefixed UTF-8 string                     |
| OS                        | 1+N    | Length-prefixed UTF-8 string                     |
| Arch                      | 1+N    | Length-prefixed UTF-8 string                     |
| Version                   | 1+N    | Length-prefixed UTF-8 string                     |
| StartTime                 | 8      | Unix timestamp (uint64)                          |
| IPCount                   | 1      | Number of IP addresses                           |
| IPAddresses[]             | 1+N ea | Length-prefixed strings (one per IPCount)         |
| PeerCount                 | 1      | Number of connected peers                        |
| Peers[]                   | varies | Per peer: PeerID(16) + Transport(1+N)            |
|                           |        |   + RTTMs(8) + IsDialer(1)                       |
| PublicKey                 | 32     | X25519 public key for E2E encryption             |
| UDPEnabled *              | 1      | 0x00 = disabled, 0x01 = enabled                  |
| ForwardListenerCount *    | 1      | Number of forward listeners                      |
| ForwardListeners[] *      | varies | Per listener: Key(1+N) + Address(1+N)            |
| ShellCount *              | 1      | Number of shell commands                         |
| Shells[] *                | 1+N ea | Length-prefixed strings (whitelisted commands)    |
| FileTransferEnabled *     | 1      | 0x00 = disabled, 0x01 = enabled                  |
+---------------------------+--------+--------------------------------------------------+

* Optional fields -- guarded by remaining-bytes check in decoder for backward
  compatibility with older agents. Fields after PublicKey are decoded only if
  bytes remain in the buffer.
```

Source: `internal/protocol/frame.go` -- `EncodeNodeInfo()` / `DecodeNodeInfo()`

---

## 6.5 UDP Relay Protocol

SOCKS5 UDP ASSOCIATE (RFC 1928) enables tunneling UDP traffic through the mesh network.

### UDP Association Lifecycle

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                     UDP ASSOCIATION LIFECYCLE                                │
│                                                                              │
│  Client              Ingress              Transit             Exit           │
│    │                    │                    │                  │            │
│    │ UDP ASSOCIATE      │                    │                  │            │
│    │───────────────────>│                    │                  │            │
│    │                    │                    │                  │            │
│    │  Reply (relay addr)│                    │                  │            │
│    │<───────────────────│                    │                  │            │
│    │                    │                    │                  │            │
│    │                    │    UDP_OPEN        │                  │            │
│    │                    │───────────────────>│   UDP_OPEN       │            │
│    │                    │                    │─────────────────>│            │
│    │                    │                    │                  │            │
│    │                    │                    │   UDP_OPEN_ACK   │            │
│    │                    │   UDP_OPEN_ACK     │<─────────────────│            │
│    │                    │<───────────────────│                  │            │
│    │                    │                    │                  │            │
│    │ UDP datagram       │                    │                  │            │
│    │~~~~~~~~~~~~~~~~~~~>│  UDP_DATAGRAM      │  UDP_DATAGRAM    │ UDP packet │
│    │                    │~~~~~~~~~~~~~~~~~~~>│~~~~~~~~~~~~~~~~~>│~~~~~~~~~~~>│
│    │                    │                    │                  │            │
│    │ UDP response       │                    │                  │ UDP reply  │
│    │<~~~~~~~~~~~~~~~~~~~│  UDP_DATAGRAM      │  UDP_DATAGRAM    │<~~~~~~~~~~~│
│    │                    │<~~~~~~~~~~~~~~~~~~~│<~~~~~~~~~~~~~~~~~│            │
│    │                    │                    │                  │            │
│    │ TCP close          │                    │                  │            │
│    │───────────────────>│   UDP_CLOSE        │   UDP_CLOSE      │            │
│    │                    │───────────────────>│─────────────────>│            │
│                                                                              │
│  Legend: ───> = TCP/Control  ~~~> = UDP/Datagram                             │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### UDP Payload Definitions

#### UDP_OPEN (0x30)

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Stable correlation ID across hops        │
│ AddressType     │ 1      │ 0x01=IPv4, 0x02=IPv6, 0x03=Domain        │
│ Address         │ varies │ Destination address (4/16/1+N bytes)     │
│ Port            │ 2      │ Destination port                         │
│ TTL             │ 1      │ Hop limit                                │
│ PathLen         │ 1      │ Number of remaining hops                 │
│ RemainingPath   │ 16*N   │ AgentIDs of remaining path nodes         │
│ EphemeralPubKey │ 32     │ X25519 public key for E2E encryption     │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### UDP_OPEN_ACK (0x31)

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Correlation ID from UDP_OPEN             │
│ AddressType     │ 1      │ Relay address type                       │
│ Address         │ varies │ Relay bind address                       │
│ Port            │ 2      │ Relay bind port                          │
│ EphemeralPubKey │ 32     │ X25519 public key for E2E encryption     │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### UDP_OPEN_ERR (0x32)

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Correlation ID from UDP_OPEN             │
│ ErrorCode       │ 2      │ Error code (see error codes table)       │
│ MessageLen      │ 1      │ Length of error message                  │
│ Message         │ N      │ Human-readable error message             │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### UDP_DATAGRAM (0x33)

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ AddressType     │ 1      │ Target address type                      │
│ Address         │ varies │ Target IP or domain                      │
│ Port            │ 2      │ Target port                              │
│ DataLen         │ 2      │ Length of encrypted data                 │
│ Data            │ varies │ Encrypted UDP payload (E2E ChaCha20)     │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### UDP_CLOSE (0x34)

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ Reason          │ 1      │ Close reason:                            │
│                 │        │   0 = Normal termination                 │
│                 │        │   1 = Idle timeout                       │
│                 │        │   2 = Error occurred                     │
│                 │        │   3 = TCP control connection closed      │
│                 │        │   4 = Administrative close               │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

### Design Notes

- **Fragmentation**: Not supported. Datagrams with frag > 0 are rejected.
- **Max Datagram Size**: 1472 bytes (MTU - IP/UDP headers)
- **Association Lifetime**: Tied to TCP control connection. When TCP closes, UDP association terminates.
- **Access Control**: Uses CIDR-based exit routes (same as TCP streams).
- **Authentication**: Uses existing SOCKS5 authentication (not separate password).

---

## 6.6 Port Forwarding (Reverse Tunnel)

Port forwarding enables reverse tunneling through the mesh - exposing local services to remote agents. Unlike SOCKS5 (outbound: local client -> mesh -> remote destination), port forwarding routes inbound traffic (remote client -> mesh -> local service).

### Architecture

**Components:**

- **Endpoint** (`internal/forward/handler.go`): Exit point where local services are exposed
- **Listener** (`internal/forward/listener.go`): Ingress point accepting remote connections
- **Routing Key**: String identifier matching listeners to endpoints

**Traffic Flow:**

```
Remote Client
    |
    | TCP connect
    |
    v
Listener Agent (forward.listeners)
    |
    | DialForward("routing-key")
    | Route lookup -> ForwardRoute
    |
    v
Transit Agent(s)
    |
    | Frame relay (E2E encrypted)
    |
    v
Endpoint Agent (forward.endpoints)
    |
    | TCP dial target
    |
    v
Local Service (target: "localhost:80")
```

### Frame Protocol

Port forwarding uses standard `STREAM_OPEN` frames with special address format:

**Address Encoding:**

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ AddressType     │ 1      │ 0x03 (AddrTypeDomain)                    │
│ AddressLength   │ 1      │ Length of address string                 │
│ Address         │ varies │ "forward:<routing-key>" (ASCII)          │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

**Protocol Constant:**

```go
const ForwardStreamPrefix = "forward:"
```

Example: For routing key "tools", the address bytes are: `\x03\x0eforward:tools`

### Route Advertisement

Forward routes propagate via flood routing (same mechanism as CIDR routes):

**ForwardRoute Structure:**

```go
type ForwardRoute struct {
    Key         string    // Routing key
    Target      string    // Endpoint target (host:port)
    OriginAgent AgentID   // Agent advertising endpoint
    NextHop     AgentID   // Next hop toward origin
    Metric      uint8     // Hop count
    Sequence    uint64    // Advertisement sequence number
    ExpiresAt   time.Time // Route TTL
    Path        []AgentID // Full path (for loop prevention)
}
```

**Advertisement Process:**

1. Endpoint agent registers local route via `AddLocalForwardRoute(key, target, 0)`
2. Route included in periodic flood advertisements
3. Transit agents increment metric and re-advertise
4. Listener agents store routes in forward routing table
5. On connection, listener performs `LookupForward(key)` to find best route

### E2E Encryption

Each forwarded connection establishes independent E2E encryption:

1. Listener generates ephemeral X25519 keypair
2. `STREAM_OPEN` includes listener's public key
3. Endpoint generates ephemeral keypair, derives shared secret via ECDH
4. `STREAM_OPEN_ACK` includes endpoint's public key
5. Both derive session key: `DeriveSessionKey(sharedSecret, listenerPub, endpointPub)`
6. All `STREAM_DATA` encrypted with ChaCha20-Poly1305

Transit agents relay encrypted frames without decryption capability.

### Configuration

**Endpoints (where service runs):**

```yaml
forward:
  endpoints:
    - key: "web-server"        # Routing key
      target: "localhost:3000" # Local service
```

**Listeners (where clients connect):**

```yaml
forward:
  listeners:
    - key: "web-server"        # Must match endpoint key
      address: ":8080"         # Bind address
      max_connections: 100     # Optional limit
```

### Error Codes

| Code | Name | Description |
|------|------|-------------|
| 40 | `ErrForwardNotFound` | Routing key not configured on endpoint |
| 41 | `ErrConnectionRefused` | Target service refused connection |
| 42 | `ErrConnectionTimeout` | Target dial timed out |

### Package Structure

```
internal/forward/
├── forward.go        # Endpoint struct, ForwardDialer interface
├── handler.go        # Exit point handler (processes STREAM_OPEN for forward)
├── listener.go       # TCP listener (accepts connections, calls DialForward)
├── handler_test.go   # Handler unit tests
└── listener_test.go  # Listener unit tests
```

---

## 6.7 ICMP Echo Protocol

The ICMP echo (ping) feature allows sending ICMP echo requests through the mesh network to test connectivity to remote hosts. Unlike TCP streams which open connections directly, ICMP uses a session-based model with dedicated frame types and unprivileged ICMP sockets.

### Architecture

**Components:**

- **Handler** (`internal/icmp/handler.go`): Exit node ICMP processing, session management, sends real echo requests
- **Session** (`internal/icmp/session.go`): Per-session state with E2E encryption keys
- **Socket** (`internal/icmp/socket.go`): Platform-specific unprivileged ICMP socket operations
- **Config** (`internal/icmp/config.go`): Configuration and CIDR validation

### Session Lifecycle

ICMP uses a session model similar to TCP streams:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        ICMP SESSION LIFECYCLE                                │
│                                                                              │
│  Client              Ingress              Transit             Exit           │
│    │                    │                    │                  │            │
│    │  Ping request      │                    │                  │            │
│    │───────────────────>│                    │                  │            │
│    │                    │                    │                  │            │
│    │                    │    ICMP_OPEN       │                  │            │
│    │                    │───────────────────>│   ICMP_OPEN      │            │
│    │                    │                    │─────────────────>│            │
│    │                    │                    │                  │            │
│    │                    │                    │   ICMP_OPEN_ACK  │            │
│    │                    │   ICMP_OPEN_ACK    │<─────────────────│            │
│    │                    │<───────────────────│                  │            │
│    │                    │                    │                  │            │
│    │                    │    ICMP_ECHO       │   ICMP_ECHO      │ Real ICMP  │
│    │                    │~~~~~~~~~~~~~~~~~~~>│~~~~~~~~~~~~~~~~~>│~~~~~~~~~~~>│
│    │                    │                    │                  │            │
│    │                    │    ICMP_ECHO       │   ICMP_ECHO      │ Real reply │
│    │  Ping response     │<~~~~~~~~~~~~~~~~~~~│<~~~~~~~~~~~~~~~~~│<~~~~~~~~~~~│
│    │<───────────────────│                    │                  │            │
│    │                    │                    │                  │            │
│    │                    │    ICMP_CLOSE      │   ICMP_CLOSE     │            │
│    │                    │───────────────────>│─────────────────>│            │
│                                                                              │
│  Legend: ───> = Control frames  ~~~> = Echo data frames                      │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Session States:**
- `StateOpening`: Session being established, awaiting ICMP_OPEN_ACK
- `StateOpen`: Active session, can relay echo packets bidirectionally
- `StateClosed`: Session terminated

### Frame Protocol

#### ICMP_OPEN (0x40)

Request to establish an ICMP echo session to a target IP.

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Stable correlation ID across hops        │
│ DestIPLen       │ 1      │ Destination IP length (4=IPv4, 16=IPv6)  │
│ DestIP          │ 4/16   │ Target IPv4 or IPv6 address              │
│ TTL             │ 1      │ Hop limit (decremented each hop)         │
│ PathLen         │ 1      │ Number of remaining hops in path         │
│ RemainingPath   │ 16*N   │ AgentIDs for remaining path              │
│ EphemeralPubKey │ 32     │ X25519 public key for E2E encryption     │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### ICMP_OPEN_ACK (0x41)

Confirms ICMP session established at exit node.

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Matches RequestID from ICMP_OPEN         │
│ EphemeralPubKey │ 32     │ Exit's X25519 public key for E2E         │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### ICMP_OPEN_ERR (0x42)

ICMP session establishment failed.

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ RequestID       │ 8      │ Matches RequestID from ICMP_OPEN         │
│ ErrorCode       │ 2      │ Error code (50, 51, 52, or 18)           │
│ MessageLen      │ 1      │ Length of error message (max 255)        │
│ Message         │ N      │ Human-readable error description         │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### ICMP_ECHO (0x43)

Carries ICMP echo request or reply data. Used bidirectionally.

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ Identifier      │ 2      │ ICMP echo identifier                     │
│ Sequence        │ 2      │ ICMP sequence number                     │
│ IsReply         │ 1      │ 0 = request, 1 = reply                   │
│ DataLen         │ 2      │ Length of encrypted payload              │
│ Data            │ N      │ Encrypted echo payload (max 1472 bytes)  │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

#### ICMP_CLOSE (0x44)

Terminates an ICMP session.

```
┌─────────────────┬────────┬──────────────────────────────────────────┐
│ Field           │ Size   │ Description                              │
├─────────────────┼────────┼──────────────────────────────────────────┤
│ Reason          │ 1      │ Close reason:                            │
│                 │        │   0 = Normal termination                 │
│                 │        │   1 = Idle timeout                       │
│                 │        │   2 = Error occurred                     │
└─────────────────┴────────┴──────────────────────────────────────────┘
```

### E2E Encryption

ICMP echo uses the same E2E encryption as TCP streams:

1. Ingress generates ephemeral X25519 keypair
2. `ICMP_OPEN` includes ingress ephemeral public key
3. Exit generates ephemeral keypair, performs ECDH key exchange
4. `ICMP_OPEN_ACK` includes exit's ephemeral public key
5. Both sides derive session key: `DeriveSessionKey(sharedSecret, requestID, ingressPub, exitPub, isInitiator)`
6. `ICMP_ECHO` data payloads encrypted with ChaCha20-Poly1305

Transit agents relay encrypted frames without decryption capability.

### Unprivileged ICMP Sockets

The implementation uses unprivileged ICMP sockets (no root required):

- **Linux**: Uses `udp4`/`udp6` network with `golang.org/x/net/icmp` package
  - Requires sysctl: `net.ipv4.ping_group_range="0 65535"` (disabled by default on most distros)
- **macOS/BSD**: Unprivileged ICMP available by default (no configuration required)
- **Windows**: Not supported (Windows lacks unprivileged ICMP socket support)

### Configuration

```yaml
icmp:
  enabled: true              # Enable/disable ICMP feature
  max_sessions: 100          # Max concurrent sessions (0=unlimited)
  idle_timeout: 60s          # Session cleanup timeout
  echo_timeout: 5s           # Per-echo reply timeout
```

### Error Codes

| Code | Name                  | Description                           |
|------|-----------------------|---------------------------------------|
| 50   | ICMP_DISABLED         | ICMP feature is disabled              |
| 52   | ICMP_SESSION_LIMIT    | Max concurrent sessions reached       |
| 18   | GENERAL_FAILURE       | Socket creation or key exchange error |

### Package Structure

```
internal/icmp/
├── handler.go       # Exit node ICMP processing, session management
├── session.go       # Session state and lifecycle
├── socket.go        # Platform-specific ICMP socket operations
├── config.go        # Configuration structure
├── handler_test.go  # Handler unit tests
└── session_test.go  # Session unit tests
```

---

## 6.8 Sleep Mode Protocol

Sleep mode enables mesh hibernation - agents can close all peer connections and enter an idle state, periodically polling for queued messages with randomized timing.

### Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SLEEP MODE STATES                                  │
│                                                                             │
│  ┌─────────┐        Sleep()        ┌──────────┐       Poll Timer      ┌────────┐
│  │  AWAKE  │ ─────────────────────►│ SLEEPING │ ────────────────────► │POLLING │
│  │         │                       │          │                       │        │
│  │ Normal  │ ◄─────────────────────│   Idle   │ ◄──────────────────── │ Brief  │
│  │ Operation│        Wake()        │ No conns │    Poll Duration      │Reconnect
│  └─────────┘                       └──────────┘                       └────────┘
│                                                                             │
│  State Transitions:                                                         │
│  • AWAKE -> SLEEPING:  Sleep() called, all connections closed               │
│  • SLEEPING -> POLLING: Poll timer fires (jittered interval)                │
│  • POLLING -> SLEEPING: Poll duration ends, connections closed              │
│  • SLEEPING/POLLING -> AWAKE: Wake() called, connections restored           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Frame Types

| Type | Value | Direction | Description |
|------|-------|-----------|-------------|
| SLEEP_COMMAND | 0x50 | Flooded | Instructs mesh to enter sleep mode |
| WAKE_COMMAND | 0x51 | Flooded | Instructs mesh to exit sleep mode |
| QUEUED_STATE | 0x52 | Point-to-point | Delivers queued state to reconnecting agent |

### Frame Formats

#### SLEEP_COMMAND (0x50)

Flooded through the mesh to instruct all agents to hibernate.

```
┌─────────────────────────────────────────────────────────────────┐
│                      SLEEP_COMMAND Frame                        │
├─────────────────────────────────────────────────────────────────┤
│ OriginAgent   │ 16 bytes │ Agent that initiated the command    │
├─────────────────────────────────────────────────────────────────┤
│ CommandID     │ 8 bytes  │ Unique ID for deduplication         │
├─────────────────────────────────────────────────────────────────┤
│ Timestamp     │ 8 bytes  │ Unix timestamp when issued          │
├─────────────────────────────────────────────────────────────────┤
│ Signature     │ 64 bytes │ Ed25519 signature (zeros if unsigned)│
├─────────────────────────────────────────────────────────────────┤
│ SeenByCount   │ 1 byte   │ Number of agents in SeenBy list     │
├─────────────────────────────────────────────────────────────────┤
│ SeenBy[]      │ Variable │ Agent IDs for loop prevention       │
└─────────────────────────────────────────────────────────────────┘
```

#### WAKE_COMMAND (0x51)

Identical structure to SLEEP_COMMAND, instructs agents to resume normal operation.

#### QUEUED_STATE (0x52)

Sent to agents when they reconnect after sleeping, containing accumulated state updates.

```
┌─────────────────────────────────────────────────────────────────┐
│                       QUEUED_STATE Frame                        │
├─────────────────────────────────────────────────────────────────┤
│ RouteCount    │ 2 bytes  │ Number of route advertisements      │
├─────────────────────────────────────────────────────────────────┤
│ Routes[]      │ Variable │ Length-prefixed RouteAdvertise[]    │
├─────────────────────────────────────────────────────────────────┤
│ WithdrawCount │ 2 bytes  │ Number of route withdrawals         │
├─────────────────────────────────────────────────────────────────┤
│ Withdraws[]   │ Variable │ Length-prefixed RouteWithdraw[]     │
├─────────────────────────────────────────────────────────────────┤
│ NodeInfoCount │ 2 bytes  │ Number of node info updates         │
├─────────────────────────────────────────────────────────────────┤
│ NodeInfos[]   │ Variable │ Length-prefixed NodeInfoAdvertise[] │
├─────────────────────────────────────────────────────────────────┤
│ HasSleepCmd   │ 1 byte   │ 1 if SleepCommand present           │
├─────────────────────────────────────────────────────────────────┤
│ SleepCmd      │ Variable │ SleepCommand (if HasSleepCmd=1)     │
├─────────────────────────────────────────────────────────────────┤
│ HasWakeCmd    │ 1 byte   │ 1 if WakeCommand present            │
├─────────────────────────────────────────────────────────────────┤
│ WakeCmd       │ Variable │ WakeCommand (if HasWakeCmd=1)       │
└─────────────────────────────────────────────────────────────────┘
```

### Sleep/Wake Command Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SLEEP COMMAND PROPAGATION                           │
│                                                                             │
│  Operator triggers sleep via CLI or HTTP API on Agent1                      │
│                                                                             │
│  Agent1                    Agent2                    Agent3                 │
│    │                         │                         │                    │
│    │──SLEEP_COMMAND─────────►│                         │                    │
│    │  (SeenBy: [A1])         │──SLEEP_COMMAND─────────►│                    │
│    │                         │  (SeenBy: [A1,A2])      │                    │
│    │                         │                         │                    │
│    │    [Enter Sleep]        │    [Enter Sleep]        │    [Enter Sleep]  │
│    │                         │                         │                    │
│    X─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ X─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ X                    │
│       (connections closed)      (connections closed)                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Poll Cycle

During sleep mode, agents periodically reconnect to receive queued state updates:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              POLL CYCLE                                     │
│                                                                             │
│  Time    │ Agent State │ Actions                                            │
│  ────────┼─────────────┼──────────────────────────────────────────────────  │
│  T+0     │ SLEEPING    │ Waiting (jittered interval: 5min +/- 30%)          │
│  T+4.2m  │ POLLING     │ Poll timer fires, reconnect to peers               │
│  T+4.2m  │ POLLING     │ Receive queued RouteAdvertise, NodeInfo            │
│  T+4.2m  │ POLLING     │ Check for WAKE_COMMAND in queued state             │
│  T+4.7m  │ SLEEPING    │ Poll duration (30s) ends, disconnect               │
│  T+4.7m  │ SLEEPING    │ Schedule next poll (jittered)                      │
│  ...     │             │                                                    │
│                                                                             │
│  Jitter prevents predictable connection patterns                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### State Queue

Awake agents queue state updates for sleeping peers:

- **Route Advertisements**: Queued with deduplication (same origin + sequence)
- **Route Withdrawals**: Queued with deduplication
- **Node Info Updates**: Latest per-origin only (older superseded)
- **Sleep/Wake Commands**: Latest command retained

Queue limits prevent memory exhaustion (configurable `max_queued_messages`).

### Command Deduplication

Sleep and wake commands use a seen-cache to prevent duplicate processing:

- Key: `(OriginAgent, CommandID)`
- TTL: 30 minutes
- Commands already seen are silently dropped

### Configuration

```yaml
sleep:
  enabled: true
  poll_interval: 5m          # Base interval between polls
  poll_interval_jitter: 0.3  # +/- 30% variation
  poll_duration: 30s         # How long to stay connected during poll
  persist_state: true        # Survive agent restarts
  max_queued_messages: 1000  # Queue limit per peer
  auto_sleep_on_start: false # Start in sleep mode
```

### State Persistence

When `persist_state` is enabled, sleep state is saved to `sleep_state.json`:

```json
{
  "state": 1,
  "sleep_start_time": "2026-01-19T10:30:00Z",
  "last_poll_time": "2026-01-19T10:35:00Z",
  "command_seq": 5
}
```

This allows agents to resume sleep mode after restart without losing context.

### Security Considerations

- Sleep/wake commands flood to all mesh agents - use signed commands in untrusted meshes
- Poll timing jitter helps avoid traffic analysis detection
- Persistent state is stored unencrypted - protect the data directory
- Signing private keys should only be distributed to authorized operators

### Command Signing and Verification

Sleep and wake commands support Ed25519 cryptographic signatures to prevent unauthorized mesh hibernation. This is separate from the X25519 keys used for stream encryption.

#### Signable Bytes Format

Commands are signed over a fixed 32-byte payload:

```
┌─────────────────────────────────────────────────────────────────┐
│                       SignableBytes (32 bytes)                  │
├─────────────────────────────────────────────────────────────────┤
│ OriginAgent   │ 16 bytes │ Agent ID that originated the command│
├─────────────────────────────────────────────────────────────────┤
│ CommandID     │ 8 bytes  │ Unique command identifier           │
├─────────────────────────────────────────────────────────────────┤
│ Timestamp     │ 8 bytes  │ Unix timestamp (nanoseconds)        │
└─────────────────────────────────────────────────────────────────┘
```

#### Signature Generation

When the operator has a signing private key configured:

1. Construct SignableBytes from command fields (32 bytes)
2. Generate Ed25519 signature using the 64-byte private key
3. Include 64-byte signature in the frame

#### Signature Verification

When an agent receives a sleep/wake command:

1. Check if `signing_public_key` is configured
2. If not configured: accept all commands (backward compatible)
3. If configured:
   - Extract SignableBytes from the command (32 bytes)
   - Verify Ed25519 signature using the 32-byte public key
   - Reject commands with invalid signatures (logged at warn level)

#### Replay Protection

Commands include a Unix timestamp (nanosecond precision). Agents verify timestamps fall within a 5-minute window of the current time. Commands with timestamps outside this window are rejected to prevent replay attacks.

#### Key Configuration

```yaml
management:
  # Signing keys (for sleep/wake command authentication)
  signing_public_key: "..."   # 64 hex chars (32 bytes) - ALL agents
  signing_private_key: "..."  # 128 hex chars (64 bytes) - OPERATORS ONLY
```

#### CLI Commands

```bash
# Generate Ed25519 signing keypair
muti-metroo signing-key generate

# Derive public key from private key
muti-metroo signing-key public
```

### Deterministic Listening Windows

To enable coordinated reconnection during sleep mode, agents can calculate when other agents will be listening using deterministic window calculations. This allows an active agent to know precisely when a sleeping peer will open its listening window.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    DETERMINISTIC LISTENING WINDOWS                          │
│                                                                             │
│  CycleLength: 5 minutes                                                     │
│  WindowLength: 30 seconds                                                   │
│  Offset: Derived from AgentID (deterministic, unique per agent)             │
│                                                                             │
│  Timeline for Agent X (offset = 2m):                                        │
│  ├────────────┬────────────┬────────────┬────────────┬────────────┤         │
│  0m           2m        2m30s         5m          7m        7m30s          │
│  │            │==LISTEN===│            │           │==LISTEN===│            │
│  │            ▲           ▲            │           ▲           ▲            │
│  │        WindowStart  WindowEnd      │       WindowStart  WindowEnd       │
│  │                                     │                                    │
│  │◄──────── Cycle 1 ─────────────────►│◄──────── Cycle 2 ──────────────►│  │
│                                                                             │
│  Clock Tolerance: +/- 5 seconds around window for connection attempts       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**WindowCalculator API:**

```go
// Calculate next listening window for a remote agent
calc := sleep.NewWindowCalculator(sleep.DefaultWindowConfig())
info := calc.GetWindowInfo(remoteAgentID, time.Now())

// info.Start, info.End - exact window boundaries
// info.SafeStart, info.SafeEnd - with clock tolerance
// info.TimeUntil - duration until window opens
// info.CurrentlyActive - true if currently in window
```

**Use Cases:**

- **Awake agent connecting to sleeping peer**: Calculate when peer's window opens, connect at optimal time
- **Mesh coordination**: Agents can predict each other's availability without explicit communication
- **Wake command delivery**: Route wake commands through agents whose windows overlap

### Package Structure

```
internal/sleep/
├── sleep.go        # Manager state machine, callbacks, persistence
├── queue.go        # StateQueue for sleeping peer message queuing
├── window.go       # Deterministic listening window calculator
├── sleep_test.go   # Sleep manager unit tests
└── window_test.go  # Window calculator unit tests
```

---

## 7. Stream Management

### 7.1 Stream Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          STREAM STATE MACHINE                               │
│                                                                             │
│                              ┌──────────┐                                   │
│                              │  CLOSED  │                                   │
│                              └────┬─────┘                                   │
│                                   │                                         │
│                    ┌──────────────┴──────────────┐                          │
│                    │                             │                          │
│               Send OPEN                     Recv OPEN                       │
│                    │                             │                          │
│                    ▼                             ▼                          │
│             ┌────────────┐                ┌────────────┐                    │
│             │  OPENING   │                │  OPENING   │                    │
│             │  (local)   │                │  (remote)  │                    │
│             └─────┬──────┘                └─────┬──────┘                    │
│                   │                             │                           │
│              Recv ACK                      Send ACK                         │
│                   │                             │                           │
│                   └──────────────┬──────────────┘                           │
│                                  ▼                                          │
│                           ┌────────────┐                                    │
│                       ┌──►│    OPEN    │◄──┐                                │
│                       │   └─────┬──────┘   │                                │
│                       │         │          │                                │
│                  STREAM_DATA    │     STREAM_DATA                           │
│                  (both ways)    │     (both ways)                           │
│                       │         │          │                                │
│                       └─────────┼──────────┘                                │
│                                 │                                           │
│              ┌──────────────────┼──────────────────┐                        │
│              │                  │                  │                        │
│         Recv CLOSE         Send CLOSE         STREAM_RESET                  │
│         (FIN_WRITE)        (FIN_WRITE)        (either side)                 │
│              │                  │                  │                        │
│              ▼                  ▼                  │                        │
│       ┌─────────────┐   ┌─────────────┐            │                        │
│       │ HALF_CLOSED │   │ HALF_CLOSED │            │                        │
│       │  (remote)   │   │   (local)   │            │                        │
│       └──────┬──────┘   └──────┬──────┘            │                        │
│              │                  │                  │                        │
│        Send CLOSE         Recv CLOSE               │                        │
│        (FIN_WRITE)        (FIN_WRITE)              │                        │
│              │                  │                  │                        │
│              └──────────────────┴──────────────────┘                        │
│                                │                                            │
│                                ▼                                            │
│                          ┌──────────┐                                       │
│                          │  CLOSED  │                                       │
│                          └──────────┘                                       │
│                                                                             │
│  Implementation Note: Half-closed states are tracked directionally as:      │
│  • StateHalfClosedLocal  - Local side initiated half-close                  │
│  • StateHalfClosedRemote - Remote side initiated half-close                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Half-Close Semantics

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          HALF-CLOSE SEMANTICS                               │
│                                                                             │
│  Half-close allows one side to signal "done sending" while still receiving. │
│  Critical for protocols like SSH that use TCP FIN for session termination.  │
│                                                                             │
│  Example: SSH session exit                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  SSH Client           Mesh Network           SSH Server             │    │
│  │       │                    │                      │                 │    │
│  │       │                    │         "exit"       │                 │    │
│  │       │                    │◄─────────────────────│                 │    │
│  │       │   STREAM_DATA      │                      │                 │    │
│  │       │◄───────────────────│                      │                 │    │
│  │       │                    │    Exit status + FIN │                 │    │
│  │       │                    │◄─────────────────────│                 │    │
│  │       │   STREAM_DATA      │                      │                 │    │
│  │       │◄───────────────────│                      │                 │    │
│  │       │   STREAM_CLOSE     │                      │                 │    │
│  │       │   (FIN_WRITE)      │                      │                 │    │
│  │       │◄───────────────────│                      │                 │    │
│  │       │                    │                      │                 │    │
│  │       │   Client processes final output           │                 │    │
│  │       │   STREAM_CLOSE     │                      │                 │    │
│  │       │   (FIN_WRITE)      │                      │                 │    │
│  │       │───────────────────►│──────────────────────►                 │    │
│  │       │                    │                      │                 │    │
│  │       │        Stream fully closed                │                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Without half-close: Server "close" would immediately kill stream,          │
│  potentially losing final output and exit status.                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.3 Resource Limits

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           RESOURCE LIMITS                                   │
│                                                                             │
│  Default limits (configurable):                                             │
│                                                                             │
│  ┌─────────────────────────────┬────────────┬───────────────────────────┐   │
│  │ Resource                    │ Default    │ Notes                     │   │
│  ├─────────────────────────────┼────────────┼───────────────────────────┤   │
│  │ Max streams per peer        │ 1,000      │ Incoming + outgoing       │   │
│  │ Max total streams           │ 10,000     │ All peers combined        │   │
│  │ Max pending opens           │ 100        │ STREAM_OPEN awaiting ACK  │   │
│  │ Stream open timeout         │ 30s        │ Time to receive ACK       │   │
│  │ Idle stream timeout         │ 5m         │ No data exchanged         │   │
│  │ Read buffer per stream      │ 256 KB     │                           │   │
│  │ Write buffer per stream     │ 256 KB     │                           │   │
│  └─────────────────────────────┴────────────┴───────────────────────────┘   │
│                                                                             │
│  When limits are reached:                                                   │
│  • New STREAM_OPEN receives STREAM_OPEN_ERR with RESOURCE_LIMIT             │
│  • Existing streams are not affected                                        │
│  • Log warning for monitoring                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.4 Proxy Chain Performance Characteristics

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                  PROXY CHAIN PERFORMANCE CHARACTERISTICS                    │
│                                                                             │
│  Note: max_hops only limits route advertisement propagation, NOT stream     │
│  path length. Stream paths are limited by the 30-second open timeout.       │
│                                                                             │
│  Recommended max hops by use case:                                          │
│  ┌────────────────────┬──────────────┬─────────────────────────────────┐    │
│  │ Use Case           │ Max Hops     │ Limiting Factor                 │    │
│  ├────────────────────┼──────────────┼─────────────────────────────────┤    │
│  │ Interactive SSH    │ 8-12 hops    │ Latency (5-50ms per hop)        │    │
│  │ Video Streaming    │ 6-10 hops    │ Buffering (256KB x hops)        │    │
│  │ Bulk Transfer      │ 12-16 hops   │ Throughput (16KB chunks)        │    │
│  │ High-latency WAN   │ 4-6 hops     │ 30s stream open timeout         │    │
│  └────────────────────┴──────────────┴─────────────────────────────────┘    │
│                                                                             │
│  Per-hop overhead:                                                          │
│  ┌────────────────────┬─────────────────────────────────────────────────┐   │
│  │ Latency            │ +1-5ms (LAN), +50-200ms (WAN)                   │   │
│  │ Memory             │ +256KB buffer per active stream                 │   │
│  │ CPU                │ Frame decode/encode at each relay               │   │
│  └────────────────────┴─────────────────────────────────────────────────┘   │
│                                                                             │
│  Protocol constants (non-configurable):                                     │
│  ┌────────────────────┬─────────────────────────────────────────────────┐   │
│  │ Max Frame Payload  │ 16 KB                                           │   │
│  │ Max Frame Size     │ 16,398 bytes (payload + 14-byte header)         │   │
│  │ Header Size        │ 14 bytes                                        │   │
│  │ Protocol Version   │ 0x01                                            │   │
│  │ Control Stream ID  │ 0 (reserved)                                    │   │
│  └────────────────────┴─────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Routing System

### 8.1 Route Table Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          ROUTE TABLE STRUCTURE                              │
│                                                                             │
│  RouteEntry {                                                               │
│      Prefix      net.IPNet     // CIDR prefix (e.g., 10.0.0.0/8)            │
│      NextHop     AgentID       // Immediate peer to forward to              │
│      ExitAgent   AgentID       // Final agent that reaches this prefix      │
│      Path        []AgentID     // Full path from here to exit               │
│      Metric      uint16        // Hop count                                 │
│      Sequence    uint64        // From origin's advertisement               │
│      ExpiresAt   time.Time     // When this route becomes invalid           │
│  }                                                                          │
│                                                                             │
│  RouteTable {                                                               │
│      entries     []*RouteEntry // Sorted by prefix length (longest first)   │
│      byOrigin    map[AgentID][]*RouteEntry                                  │
│      lock        sync.RWMutex                                               │
│  }                                                                          │
│                                                                             │
│  Route entries are sorted by prefix length (descending) for efficient       │
│  longest-prefix match during lookup.                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Domain Route Table

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DOMAIN ROUTE TABLE STRUCTURE                         │
│                                                                             │
│  DomainRoute {                                                              │
│      Pattern     string        // "example.com" or "*.example.com"          │
│      IsWildcard  bool          // True if pattern starts with "*."          │
│      BaseDomain  string        // For wildcard: domain without "*."         │
│      NextHop     AgentID       // Immediate peer to forward to              │
│      OriginAgent AgentID       // Agent that advertised this route          │
│      Path        []AgentID     // Full path from here to origin             │
│      Metric      uint16        // Hop count                                 │
│      Sequence    uint64        // From origin's advertisement               │
│      LastUpdate  time.Time     // When this route was last updated          │
│  }                                                                          │
│                                                                             │
│  DomainTable {                                                              │
│      exactRoutes  map[string][]*DomainRoute  // "api.example.com" -> routes │
│      wildcardBase map[string][]*DomainRoute  // "example.com" -> wildcard   │
│      localID      AgentID                    // Local agent ID              │
│  }                                                                          │
│                                                                             │
│  Matching algorithm:                                                        │
│  1. Exact match: lookup domain in exactRoutes                               │
│  2. Wildcard match: split domain at first ".", lookup remainder in          │
│     wildcardBase (single-level wildcard only)                               │
│  3. Return route with lowest metric                                         │
│                                                                             │
│  Example lookups:                                                           │
│  • "api.example.com" with route "api.example.com" -> exact match            │
│  • "foo.example.com" with route "*.example.com" -> wildcard match           │
│  • "a.b.example.com" with route "*.example.com" -> NO MATCH (multi-level)   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Longest Prefix Match

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        LONGEST PREFIX MATCH                                 │
│                                                                             │
│  Example route table on Agent1:                                             │
│                                                                             │
│  ┌──────────────────┬──────────┬──────────┬────────┬────────────────────┐   │
│  │ Prefix           │ NextHop  │ Exit     │ Metric │ Path               │   │
│  ├──────────────────┼──────────┼──────────┼────────┼────────────────────┤   │
│  │ 10.5.3.0/24      │ Agent2   │ Agent3   │ 2      │ [Agent2, Agent3]   │   │
│  │ 10.5.0.0/16      │ Agent2   │ Agent4   │ 3      │ [Agent2, Agent3,   │   │
│  │                  │          │          │        │  Agent4]           │   │
│  │ 10.0.0.0/8       │ Agent2   │ Agent3   │ 2      │ [Agent2, Agent3]   │   │
│  │ 0.0.0.0/0        │ Agent5   │ Agent5   │ 1      │ [Agent5]           │   │
│  └──────────────────┴──────────┴──────────┴────────┴────────────────────┘   │
│                                                                             │
│  Lookup(10.5.3.100):                                                        │
│    Matches: 10.5.3.0/24 (24 bits) ← Selected (longest prefix)               │
│    Result: Path=[Agent2, Agent3]                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.3 Route Expiration

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          ROUTE EXPIRATION                                   │
│                                                                             │
│  Routes have a TTL and expire if not refreshed.                             │
│                                                                             │
│  Timing parameters:                                                         │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐   │
│  │ Parameter                   │ Default  │ Description                 │   │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤   │
│  │ Route TTL                   │ 5m       │ How long routes are valid   │   │
│  │ Advertise interval          │ 2m       │ How often to re-advertise   │   │
│  │ Cleanup interval            │ 30s      │ How often to check expiry   │   │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘   │
│                                                                             │
│  On peer disconnect:                                                        │
│  • Immediately remove all routes where NextHop = disconnected peer          │
│  • Don't wait for expiration                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 9. Flood Protocol

### 9.1 Route Advertisement Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       ROUTE ADVERTISEMENT FLOW                              │
│                                                                             │
│  Agent3 is exit for 10.0.0.0/8                                              │
│                                                                             │
│       Agent3                   Agent2                   Agent1              │
│          │                        │                        │                │
│          │ ROUTE_ADVERTISE        │                        │                │
│          │ origin=Agent3          │                        │                │
│          │ seq=1                  │                        │                │
│          │ routes=[10.0.0.0/8]    │                        │                │
│          │ metric=0               │                        │                │
│          │ path=[Agent3]          │                        │                │
│          │ seenBy=[Agent3]        │                        │                │
│          │───────────────────────►│                        │                │
│          │                        │                        │                │
│          │                        │ Store route:           │                │
│          │                        │ 10.0.0.0/8             │                │
│          │                        │ nextHop=Agent3         │                │
│          │                        │ exit=Agent3            │                │
│          │                        │ metric=1               │                │
│          │                        │                        │                │
│          │                        │ ROUTE_ADVERTISE        │                │
│          │                        │ origin=Agent3          │                │
│          │                        │ seq=1                  │                │
│          │                        │ metric=1               │                │
│          │                        │ path=[Agent2,Agent3]   │                │
│          │                        │ seenBy=[Agent3,Agent2] │                │
│          │                        │───────────────────────►│                │
│          │                        │                        │                │
│          │                        │                        │ Store route    │
│          │                        │                        │ metric=2       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Flood Algorithm

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FLOOD ALGORITHM                                   │
│                                                                             │
│  OnReceive(ROUTE_ADVERTISE, fromPeer):                                      │
│                                                                             │
│      // 1. Loop prevention                                                  │
│      if msg.OriginAgent == self.ID:                                         │
│          return  // Our own advertisement came back                         │
│                                                                             │
│      if self.ID in msg.SeenBy:                                              │
│          return  // Already processed this                                  │
│                                                                             │
│      // 2. Freshness check                                                  │
│      lastSeq := sequenceTracker[msg.OriginAgent]                            │
│      if msg.Sequence <= lastSeq:                                            │
│          return  // Stale update                                            │
│      sequenceTracker[msg.OriginAgent] = msg.Sequence                        │
│                                                                             │
│      // 3. Store routes                                                     │
│      for route in msg.Routes:                                               │
│          entry := RouteEntry{                                               │
│              Prefix:    route.Prefix,                                       │
│              NextHop:   fromPeer,                                           │
│              ExitAgent: msg.OriginAgent,                                    │
│              Path:      prepend(fromPeer, msg.Path),                        │
│              Metric:    route.Metric + 1,                                   │
│              Sequence:  msg.Sequence,                                       │
│              ExpiresAt: now() + routeTTL,                                   │
│          }                                                                  │
│          routeTable.Update(entry)                                           │
│                                                                             │
│      // 4. Forward to other peers (split horizon)                           │
│      msg.SeenBy = append(msg.SeenBy, self.ID)                               │
│      for peer in connectedPeers:                                            │
│          if peer != fromPeer && peer not in msg.SeenBy:                     │
│              send(peer, msg)  // Log errors, don't fail                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 10. Peer Connection Management

### 10.1 Connection State Machine

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       CONNECTION STATE MACHINE                              │
│                                                                             │
│                           ┌────────────────┐                                │
│              ┌───────────►│  DISCONNECTED  │◄───────────────┐               │
│              │            └───────┬────────┘                │               │
│              │                    │                         │               │
│              │       Dial() or Accept()                     │               │
│              │                    │                    Max retries          │
│              │                    ▼                    exceeded             │
│              │            ┌────────────────┐                │               │
│              │            │  CONNECTING    │                │               │
│              │            └───────┬────────┘                │               │
│              │                    │                         │               │
│              │         Transport connected                  │               │
│              │                    │                         │               │
│              │                    ▼                         │               │
│              │            ┌────────────────┐                │               │
│              │      ┌────►│  HANDSHAKING   │                │               │
│              │      │     └───────┬────────┘                │               │
│              │      │             │                         │               │
│              │      │    PEER_HELLO exchange OK             │               │
│              │      │             │                         │               │
│              │      │             ▼                         │               │
│              │      │     ┌────────────────┐                │               │
│           Reconnect │     │   CONNECTED    │◄───┐           │               │
│           success   │     └───────┬────────┘    │           │               │
│              │      │             │             │           │               │
│              │      │        Error/Timeout  Keepalive       │               │
│              │      │             │          success        │               │
│              │      │             ▼             │           │               │
│              │      │     ┌────────────────┐    │           │               │
│              │      └─────│  RECONNECTING  │────┘           │               │
│              │            └───────┬────────┘                │               │
│              │                    │                         │               │
│              │                    └─────────────────────────┘               │
│              │                                                              │
│         Shutdown/                                                           │
│         Give up                                                             │
│              │                                                              │
│              └──────────────────────────────────────────────────────────────│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.2 Handshake Protocol

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          HANDSHAKE PROTOCOL                                 │
│                                                                             │
│  Agent A (dialer)                              Agent B (listener)           │
│       │                                              │                      │
│       │ [Transport connects - QUIC/HTTP2/WS]         │                      │
│       │◄────────────────────────────────────────────►│                      │
│       │                                              │                      │
│       │ PEER_HELLO                                   │                      │
│       │ version=1                                    │                      │
│       │ agentID=A                                    │                      │
│       │ timestamp=1703001234                         │                      │
│       │ capabilities=["socks5"]                      │                      │
│       │─────────────────────────────────────────────►│                      │
│       │                                              │                      │
│       │                                              │ Validate:            │
│       │                                              │ • Version compatible │
│       │                                              │ • AgentID expected?  │
│       │                                              │ • Timestamp fresh?   │
│       │                                              │                      │
│       │                                   PEER_HELLO_ACK                    │
│       │                                   version=1  │                      │
│       │                                   agentID=B  │                      │
│       │                                   timestamp=1703001235              │
│       │                                   capabilities=["exit"]             │
│       │◄─────────────────────────────────────────────│                      │
│       │                                              │                      │
│       │              Connection ready                │                      │
│                                                                             │
│  Handshake timeout: 10 seconds                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Reconnection Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        RECONNECTION STRATEGY                                │
│                                                                             │
│  Exponential backoff with jitter:                                           │
│                                                                             │
│  Parameters:                                                                │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐   │
│  │ Parameter                   │ Default  │ Description                 │   │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤   │
│  │ Initial delay               │ 1s       │ First reconnect attempt     │   │
│  │ Max delay                   │ 60s      │ Cap on backoff              │   │
│  │ Multiplier                  │ 2.0      │ Exponential factor          │   │
│  │ Jitter                      │ 0.2      │ ±20% randomization          │   │
│  │ Max retries                 │ 0        │ 0 = infinite                │   │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘   │
│                                                                             │
│  Retry sequence:                                                            │
│    Attempt 1:  1s    ± 0.2s  →  0.8s  - 1.2s                                │
│    Attempt 2:  2s    ± 0.4s  →  1.6s  - 2.4s                                │
│    Attempt 3:  4s    ± 0.8s  →  3.2s  - 4.8s                                │
│    ...                                                                      │
│    Attempt 7+: 60s   ± 12s   →  48s   - 72s  (capped)                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.4 Keepalive Mechanism

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         KEEPALIVE MECHANISM                                 │
│                                                                             │
│  Purpose:                                                                   │
│  • Detect dead connections                                                  │
│  • Maintain NAT/firewall mappings                                           │
│  • Measure round-trip time                                                  │
│                                                                             │
│  Parameters:                                                                │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐   │
│  │ Parameter                   │ Default  │ Description                 │   │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤   │
│  │ Idle threshold              │ 5m       │ Send keepalive after idle   │   │
│  │ Timeout                     │ 90s      │ Declare dead if no response │   │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘   │
│                                                                             │
│  Sent on StreamID 0 (control stream).                                       │
│  KEEPALIVE_ACK echoes the same timestamp for RTT calculation.               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 11. SOCKS5 Server

### 11.1 Supported Features

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SOCKS5 SUPPORT                                      │
│                                                                             │
│  Supported:                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • CONNECT command (TCP proxy)                                       │    │
│  │ • UDP ASSOCIATE command (UDP proxy)                                 │    │
│  │ • IPv4 addresses                                                    │    │
│  │ • IPv6 addresses                                                    │    │
│  │ • Domain names (resolved at exit agent)                             │    │
│  │ • No authentication (method 0x00)                                   │    │
│  │ • Username/password authentication (method 0x02)                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Not supported:                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • BIND command (incoming connections)                               │    │
│  │ • GSSAPI authentication (method 0x01)                               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.2 Error Mapping

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SOCKS5 ERROR MAPPING                                │
│                                                                             │
│  ┌─────────────────────────────┬────────────────────────────────────────┐   │
│  │ Internal Error              │ SOCKS5 Reply                           │   │
│  ├─────────────────────────────┼────────────────────────────────────────┤   │
│  │ NO_ROUTE                    │ 0x04 Host unreachable                  │   │
│  │ CONNECTION_REFUSED          │ 0x05 Connection refused                │   │
│  │ CONNECTION_TIMEOUT          │ 0x06 TTL expired                       │   │
│  │ TTL_EXCEEDED                │ 0x06 TTL expired                       │   │
│  │ HOST_UNREACHABLE            │ 0x04 Host unreachable                  │   │
│  │ NETWORK_UNREACHABLE         │ 0x03 Network unreachable               │   │
│  │ DNS_ERROR                   │ 0x04 Host unreachable                  │   │
│  │ EXIT_DISABLED               │ 0x01 General failure                   │   │
│  │ RESOURCE_LIMIT              │ 0x01 General failure                   │   │
│  │ Any other error             │ 0x01 General failure                   │   │
│  └─────────────────────────────┴────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.3 WebSocket Transport

The SOCKS5 server can optionally accept connections over WebSocket transport, enabling tunneling through environments where raw TCP/SOCKS5 traffic is blocked but HTTPS/WebSocket is permitted.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SOCKS5 WEBSOCKET TRANSPORT                               │
│                                                                             │
│  TCP Transport (default):                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Client ──[TCP]──> 127.0.0.1:1080 ──> SOCKS5 Handler                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  WebSocket Transport (optional):                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Client ──[WebSocket]──> 0.0.0.0:8443/socks5 ──> SOCKS5 Handler      │    │
│  │                                                                     │    │
│  │ • Appears as HTTPS traffic to network filters                       │    │
│  │ • TLS termination at listener (or plaintext behind reverse proxy)  │    │
│  │ • Same SOCKS5 protocol, different transport                         │    │
│  │ • Splash page at "/" for camouflage                                 │    │
│  │ • HTTP Basic Auth when socks5.auth is enabled                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Configuration:                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ socks5:                                                             │    │
│  │   websocket:                                                        │    │
│  │     enabled: true                                                   │    │
│  │     address: "0.0.0.0:8443"   # Listen address                      │    │
│  │     path: "/socks5"           # WebSocket upgrade path              │    │
│  │     plaintext: false          # TLS (true for reverse proxy)        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

The WebSocket listener wraps websocket.Conn to implement net.Conn, allowing the same SOCKS5 handler to serve both TCP and WebSocket clients. Binary WebSocket messages carry SOCKS5 protocol data unchanged.

**Authentication Flow:**

When `socks5.auth.enabled` is true, the WebSocket endpoint enforces two layers of authentication:

1. **HTTP Basic Auth** - Validated before WebSocket upgrade. The WebSocket listener receives a `CredentialStore` from the agent and checks the `Authorization` header. Invalid or missing credentials return HTTP 401 Unauthorized.

2. **SOCKS5 Username/Password** - Standard RFC 1929 authentication after the WebSocket connection is established. The same credential store is used by the SOCKS5 handler.

Both use the same `socks5.auth.users` configuration. Compatible clients (like Mutiauk) automatically send credentials for both layers using the same username/password.

---

## 12. Data Plane

### 12.1 Frame Processing

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         FRAME PROCESSING                                    │
│                                                                             │
│  func processFrame(peer *PeerConn, frame *Frame):                           │
│      switch frame.Type:                                                     │
│                                                                             │
│      case STREAM_OPEN:                                                      │
│          handleStreamOpen(peer, frame)                                      │
│                                                                             │
│      case STREAM_OPEN_ACK, STREAM_OPEN_ERR:                                 │
│          handleStreamOpenResponse(peer, frame)                              │
│                                                                             │
│      case STREAM_DATA:                                                      │
│          entry := forwardTable.Lookup(peer.ID, frame.StreamID)              │
│          if entry == nil:                                                   │
│              send STREAM_RESET (unknown stream)                             │
│              return                                                         │
│          if entry.LocalConn != nil:                                         │
│              entry.LocalConn.Write(frame.Payload)  // Exit                  │
│          else:                                                              │
│              forward(entry.OutgoingPeer, frame)    // Transit               │
│                                                                             │
│      case STREAM_CLOSE:                                                     │
│          handleStreamClose(peer, frame)                                     │
│                                                                             │
│      case STREAM_RESET:                                                     │
│          handleStreamReset(peer, frame)                                     │
│                                                                             │
│      case ROUTE_ADVERTISE:                                                  │
│          floodProtocol.HandleAdvertise(peer, frame)                         │
│                                                                             │
│      case ROUTE_WITHDRAW:                                                   │
│          floodProtocol.HandleWithdraw(peer, frame)                          │
│                                                                             │
│      case KEEPALIVE:                                                        │
│          send KEEPALIVE_ACK with same timestamp                             │
│                                                                             │
│      case KEEPALIVE_ACK:                                                    │
│          updateRTT(peer, frame.Timestamp)                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 12.2 Write Fairness

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          WRITE FAIRNESS                                     │
│                                                                             │
│  Problem: One stream (video) could monopolize the transport, starving       │
│  latency-sensitive streams (SSH).                                           │
│                                                                             │
│  Solution for HTTP/2 and WebSocket (single transport stream):               │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  // Maximum data per frame                                          │    │
│  │  const maxFramePayload = 16384  // 16 KB                            │    │
│  │                                                                     │    │
│  │  // Writer loop with round-robin                                    │    │
│  │  for {                                                              │    │
│  │      streams := scheduler.GetReadyStreams()                         │    │
│  │      for _, stream := range streams {                               │    │
│  │          // Send at most one frame per stream per round             │    │
│  │          data := stream.queue.Read(maxFramePayload)                 │    │
│  │          frame := createDataFrame(stream.id, data)                  │    │
│  │          transport.Write(frame)                                     │    │
│  │      }                                                              │    │
│  │  }                                                                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  QUIC transport: Not needed (native per-stream fairness).                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 13. Configuration

### 13.1 Configuration File (YAML)

```yaml
# ==============================================================================
# Mesh Agent Configuration
# ==============================================================================

# ------------------------------------------------------------------------------
# Agent Identity
# ------------------------------------------------------------------------------
agent:
  # Agent ID: "auto" generates on first run, or specify hex string
  id: "auto"

  # Human-readable display name (Unicode allowed)
  # If not set, falls back to agent ID for display
  display_name: ""

  # Directory for persistent state
  data_dir: "./data"

  # Logging
  log_level: "info" # debug, info, warn, error
  log_format: "text" # text, json

# ------------------------------------------------------------------------------
# Protocol Identifiers (OPSEC)
# Customize identifiers that appear in network traffic
# Set values to empty strings to disable custom identifiers
# ------------------------------------------------------------------------------
protocol:
  alpn: "muti-metroo/1" # ALPN for QUIC/TLS (empty to disable)
  http_header: "X-Muti-Metroo-Protocol" # HTTP/2 header (empty to disable)
  ws_subprotocol: "muti-metroo/1" # WebSocket subprotocol (empty to disable)

# ------------------------------------------------------------------------------
# Transport Listeners
# ------------------------------------------------------------------------------
listeners:
  # QUIC listener (best performance)
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      # Option 1: File paths
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt" # Optional: require client certs

      # Option 2: Inline PEM content (takes precedence over file paths)
      # Useful for secrets management or single-file configs
      # cert_pem: |
      #   -----BEGIN CERTIFICATE-----
      #   ...
      #   -----END CERTIFICATE-----
      # key_pem: |
      #   -----BEGIN PRIVATE KEY-----
      #   ...
      #   -----END PRIVATE KEY-----

  # HTTP/2 listener (TCP fallback)
  - transport: h2
    address: "0.0.0.0:8443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

  # WebSocket listener (maximum compatibility)
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

# ------------------------------------------------------------------------------
# Peer Connections
# ------------------------------------------------------------------------------
peers:
  # QUIC peer
  - id: "abc123..." # Expected peer AgentID
    transport: quic
    address: "192.168.1.50:4433"
    tls:
      ca: "./certs/peer-ca.crt"
      strict: true  # Enable CA verification

  # WebSocket peer through proxy
  - id: "ghi789..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    proxy: "http://proxy.corp.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"

# ------------------------------------------------------------------------------
# SOCKS5 Server
# ------------------------------------------------------------------------------
socks5:
  enabled: true
  address: "127.0.0.1:1080"

  # Authentication (optional)
  auth:
    enabled: false
    users:
      - username: "user1"
        password: "pass1"

  # Limits
  max_connections: 1000

  # WebSocket transport (optional)
  # Enables SOCKS5 over WebSocket for environments where raw TCP is blocked
  websocket:
    enabled: false
    address: "0.0.0.0:8443"  # Listen address for WebSocket connections
    path: "/socks5"          # WebSocket upgrade path
    plaintext: false         # Set true when behind reverse proxy (nginx/Caddy)

# ------------------------------------------------------------------------------
# Exit Configuration
# ------------------------------------------------------------------------------
exit:
  enabled: true

  # CIDR routes to advertise
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"

  # DNS settings
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s

# ------------------------------------------------------------------------------
# Routing
# ------------------------------------------------------------------------------
routing:
  advertise_interval: 2m
  node_info_interval: 2m # Node info advertisement (defaults to advertise_interval)
  route_ttl: 5m
  max_hops: 16

# ------------------------------------------------------------------------------
# Connection Tuning
# ------------------------------------------------------------------------------
connections:
  # Keepalive
  idle_threshold: 5m
  timeout: 90s

  # Reconnection
  reconnect:
    initial_delay: 1s
    max_delay: 60s
    multiplier: 2.0
    jitter: 0.2
    max_retries: 0 # 0 = infinite

# ------------------------------------------------------------------------------
# Resource Limits
# ------------------------------------------------------------------------------
limits:
  max_streams_per_peer: 1000
  max_streams_total: 10000
  max_pending_opens: 100
  stream_open_timeout: 30s
  buffer_size: 262144 # 256 KB

# ------------------------------------------------------------------------------
# HTTP API Server
# ------------------------------------------------------------------------------
http:
  enabled: true
  address: ":8080"
  read_timeout: 10s
  write_timeout: 10s

  # Endpoint control flags
  minimal: false # When true, only /health, /healthz, /ready are enabled
  pprof: false # /debug/pprof/* endpoints (disable in production)
  dashboard: true # /api/* endpoints
  remote_api: true # /agents/* endpoints

# ------------------------------------------------------------------------------
# Control Socket
# ------------------------------------------------------------------------------
control:
  enabled: true
  socket_path: "./data/control.sock"

# ------------------------------------------------------------------------------
# Remote Shell
# ------------------------------------------------------------------------------
shell:
  enabled: false # Disabled by default for security
  whitelist: [] # Empty = no commands allowed; ["*"] = all (testing only!)
  password_hash: "" # bcrypt hash of shell password
  timeout: 60s # Default command execution timeout

# ------------------------------------------------------------------------------
# File Transfer
# ------------------------------------------------------------------------------
file_transfer:
  enabled: false # Disabled by default for security
  max_file_size: 524288000 # Maximum file size in bytes (500 MB)
  allowed_paths: [] # Allowed path prefixes (empty = all absolute paths)
  password_hash: "" # bcrypt hash of file transfer password

# ------------------------------------------------------------------------------
# Management Key Encryption
# Encrypt mesh topology data for OPSEC protection
# ------------------------------------------------------------------------------
management:
  # Management public key (64-character hex string)
  # When set, NodeInfo and route paths are encrypted before flooding
  # All agents in mesh should have the SAME public key
  # Generate with: muti-metroo management-key generate
  public_key: ""

  # Management private key (64-character hex string)
  # Only set on operator/management nodes that need to view topology
  # NEVER distribute to field agents
  # When set, this node can decrypt NodeInfo and view mesh topology
  private_key: ""
```

### 13.2 Environment Variable Substitution

Configuration values support environment variable substitution:

```yaml
peers:
  - id: "${PEER_ID}"
    address: "${PEER_ADDRESS}"
    proxy_auth:
      password: "${PROXY_PASSWORD:-default}"
```

Syntax:

- `${VAR}` - Substitute variable value
- `${VAR:-default}` - Use default if variable not set

### 13.3 Command-Line Interface

```bash
# Interactive setup wizard
muti-metroo setup

# Initialize new agent
muti-metroo init --data-dir ./data

# Run agent
muti-metroo run --config ./config.yaml

# Show status
muti-metroo status --socket ./data/control.sock

# List peers
muti-metroo peers

# List routes
muti-metroo routes

# Certificate management
muti-metroo cert ca -n "My CA" -o ./certs -d 365
muti-metroo cert agent -n "agent-1" --ca ./certs/ca.crt
muti-metroo cert client -n "client-1"
muti-metroo cert info ./certs/ca.crt

# Remote shell (execute command on remote agent)
muti-metroo shell <target-agent-id> <command> [args...]
muti-metroo shell -a 192.168.1.10:8080 abc123def456 hostname
muti-metroo shell -p secret abc123def456 whoami
muti-metroo shell --tty abc123def456 bash

# File transfer
muti-metroo upload <target-agent-id> <local-path> <remote-path>
muti-metroo download <target-agent-id> <remote-path> <local-path>

# Password hash generation (for SOCKS5, shell, file transfer auth)
muti-metroo hash                     # Interactive prompt
muti-metroo hash "password"          # From argument
muti-metroo hash --cost 12           # Custom bcrypt cost

# Management key encryption
muti-metroo management-key generate  # Generate keypair
muti-metroo management-key public --private <key>  # Derive public from private

# Service management
muti-metroo service install -c /path/to/config.yaml
muti-metroo service uninstall
muti-metroo service status
```

---

## 14. Security

### 14.1 Transport Security

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         TRANSPORT SECURITY                                  │
│                                                                             │
│  Layered Security Model:                                                    │
│  • Primary: E2E encryption (X25519 + ChaCha20-Poly1305) protects payloads   │
│  • Secondary: TLS 1.3 encrypts transport (defense-in-depth)                 │
│                                                                             │
│  Default behavior (no TLS config):                                          │
│  • Auto-generate self-signed ECDSA certificate on startup                   │
│  • No certificate verification (safe because E2E protects payload)          │
│  • Certificates regenerated each startup (ephemeral)                        │
│                                                                             │
│  All transports use TLS:                                                    │
│                                                                             │
│  ┌─────────────────┬─────────────────────────────────────────────────────┐  │
│  │ Transport       │ TLS Implementation                                  │  │
│  ├─────────────────┼─────────────────────────────────────────────────────┤  │
│  │ QUIC            │ TLS 1.3 (mandatory, built into QUIC)                │  │
│  │ HTTP/2          │ TLS 1.3 (MinVersion enforced)                       │  │
│  │ WebSocket       │ TLS 1.3 (WSS, MinVersion enforced)                  │  │
│  └─────────────────┴─────────────────────────────────────────────────────┘  │
│                                                                             │
│  Strict mode (tls.strict: true):                                            │
│  • CA-based: Validate against trusted CA certificate                        │
│  • Mutual TLS: Both sides present and validate certificates                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 14.2 Trust Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            TRUST MODEL                                      │
│                                                                             │
│  Direct trust:                                                              │
│  • Agent trusts directly connected peers                                    │
│  • Verified via AgentID handshake (TLS verification optional)               │
│                                                                             │
│  Transitive trust:                                                          │
│  • Agent trusts route advertisements from direct peers                      │
│  • These include routes from agents further in the mesh                     │
│  • No direct verification of distant agents                                 │
│                                                                             │
│  What transit agents can see:                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Visible:                                                            │    │
│  │ • Destination IP/port (in STREAM_OPEN)                              │    │
│  │ • Traffic volume and timing                                         │    │
│  │ • Path information                                                  │    │
│  │                                                                     │    │
│  │ Not visible (encrypted end-to-end):                                 │    │
│  │ • Payload content (encrypted with ChaCha20-Poly1305)                │    │
│  │ • Application-layer data                                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  End-to-end encryption (built-in):                                          │
│  • X25519 key exchange during stream open (ephemeral keys in frames)        │
│  • ChaCha20-Poly1305 AEAD for all STREAM_DATA payloads                      │
│  • Transit agents cannot decrypt stream data                                │
│  • Each stream has unique session keys derived via ECDH                     │
│                                                                             │
│  Key exchange flow:                                                         │
│  1. Ingress generates ephemeral X25519 keypair                              │
│  2. Ingress sends public key in STREAM_OPEN                                 │
│  3. Exit generates ephemeral X25519 keypair                                 │
│  4. Exit sends public key in STREAM_OPEN_ACK                                │
│  5. Both compute shared secret: ECDH(local_private, remote_public)          │
│  6. Session key derived: HKDF-SHA256(shared_secret)                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 14.3 Configuration Security

Sensitive configuration values are automatically redacted in logs:

```go
// Redacted fields:
// - peers[].proxy_auth.password
// - peers[].tls.key
// - peers[].tls.key_pem
// - listeners[].tls.key
// - listeners[].tls.key_pem
// - socks5.auth.users[].password
// - shell.password_hash

config.String()       // Returns YAML with [REDACTED] for sensitive values
config.StringUnsafe() // Returns full YAML (use only for debugging)
config.Redacted()     // Returns copy with redacted values
```

---

## 15. Observability

### 15.1 HTTP API Endpoints

HTTP endpoints are exposed when `http.enabled: true`:

**Health & Monitoring:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Basic liveness probe (returns "OK") |
| `/healthz` | GET | Detailed health with JSON stats |
| `/ready` | GET | Readiness probe (returns "READY") |

**JSON API:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/topology` | GET | Topology data (agents and connections) |
| `/api/dashboard` | GET | Dashboard overview (agent info, stats, peers, routes) |
| `/api/nodes` | GET | Detailed node info listing for all known agents |

**Distributed Status:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/agents` | GET | List all known agents in the mesh |
| `/agents/{agent-id}` | GET | Get status from specific agent |
| `/agents/{agent-id}/routes` | GET | Get route table from specific agent |
| `/agents/{agent-id}/peers` | GET | Get peer list from specific agent |
| `/agents/{agent-id}/shell` | GET | WebSocket shell access on remote agent |
| `/agents/{agent-id}/file/upload` | POST | Upload file to remote agent |
| `/agents/{agent-id}/file/download` | POST | Download file from remote agent |

**Management:**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/routes/advertise` | POST | Trigger immediate route advertisement |

**Debugging (pprof):**
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/debug/pprof/` | GET | pprof index |
| `/debug/pprof/cmdline` | GET | Running program's command line |
| `/debug/pprof/profile` | GET | CPU profile |
| `/debug/pprof/symbol` | GET | Symbol lookup |
| `/debug/pprof/trace` | GET | Execution trace |

**Example `/healthz` response:**

```json
{
  "status": "healthy",
  "running": true,
  "peer_count": 3,
  "stream_count": 5,
  "route_count": 10,
  "socks5_running": true,
  "exit_handler_running": false
}
```

### 15.2 Structured Logging

Logging uses Go's `slog` package with configurable levels and formats.

**Log Levels:** `debug`, `info`, `warn`, `error`

**Log Formats:** `text`, `json`

**Standard Log Fields:**

```go
const (
    KeyPeerID     = "peer_id"
    KeyStreamID   = "stream_id"
    KeyRequestID  = "request_id"
    KeyAddress    = "address"
    KeyTransport  = "transport"
    KeyRoute      = "route"
    KeyHops       = "hops"
    KeyError      = "error"
    KeyComponent  = "component"
    KeyDuration   = "duration"
)
```

---

## 16. Operations

### 16.1 Control Socket API

Unix socket HTTP API for management commands when `control.enabled: true`.

**Socket Path:** Configured via `control.socket_path` (default: `./data/control.sock`)

| Endpoint  | Method | Response                        |
| --------- | ------ | ------------------------------- |
| `/status` | GET    | Agent ID, running state, counts |
| `/peers`  | GET    | List of connected peer IDs      |
| `/routes` | GET    | Routing table entries           |

**Example `/status` response:**

```json
{
  "agent_id": "a3f8c2d1e5b94a7c8d2e1f0a3b5c7d9e",
  "running": true,
  "peer_count": 3,
  "route_count": 10
}
```

### 16.2 Service Installation

The agent can be installed as a system service.

**Supported Platforms:**

- Linux (systemd)
- macOS (launchd)
- Windows (Service Control Manager)

**Linux Installation:**

```bash
# Via setup wizard
sudo muti-metroo setup

# Manual installation handled by wizard
# Creates /etc/systemd/system/muti-metroo.service
```

**Systemd Service Features:**

- Restart on failure (5s delay)
- Security hardening (NoNewPrivileges, ProtectSystem, PrivateTmp)
- Journal logging integration

**Service Management:**

```bash
# Uninstall
sudo muti-metroo uninstall --name muti-metroo

# View status
systemctl status muti-metroo

# View logs
journalctl -u muti-metroo -f
```

**macOS Installation:**

```bash
# Via setup wizard
sudo muti-metroo setup

# Manual installation handled by wizard
# Creates /Library/LaunchDaemons/com.muti-metroo.plist
```

**macOS Service Management:**

```bash
# Load/start service
sudo launchctl load -w /Library/LaunchDaemons/com.muti-metroo.plist

# Unload/stop service
sudo launchctl unload -w /Library/LaunchDaemons/com.muti-metroo.plist

# View logs
tail -f /path/to/working/dir/muti-metroo.log
```

### 16.3 Setup Wizard

Interactive CLI wizard for initial configuration:

```bash
muti-metroo setup
```

**Wizard Steps:**

1. Basic setup (data directory, config path)
2. Agent role selection (ingress, transit, exit)
3. Network configuration (transport, address)
4. TLS setup (generate, paste, or use existing certs)
5. Peer connections
6. SOCKS5 configuration (if ingress)
7. Exit configuration (if exit)
8. Advanced options (logging, health, control)
9. Service installation (on supported platforms)

---

## 17. Certificate Management

### 17.1 Default Behavior

By default, TLS certificates are auto-generated and verification is disabled:

- Agent generates self-signed ECDSA (P-256) certificate on startup
- Certificates are ephemeral (regenerated each startup)
- No CA required, no certificate verification
- Safe because E2E encryption (X25519 + ChaCha20-Poly1305) protects payload

**Config for strict TLS:**
```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  strict: true   # Enable certificate verification
  mtls: true     # Optional: require client certificates
```

### 17.2 Certificate Types

| Type       | Usage                                     | Default Validity |
| ---------- | ----------------------------------------- | ---------------- |
| CA         | Sign other certificates                   | 365 days         |
| Agent/Peer | Server + client auth for mesh connections | 90 days          |
| Client     | Client authentication only                | 90 days          |

### 17.3 Certificate Generation

```bash
# Generate CA
muti-metroo cert ca \
  --cn "My CA" \
  --out ./certs \
  --days 365

# Generate agent certificate (signed by CA)
muti-metroo cert agent \
  --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  --dns "agent-1.local,localhost" \
  --ip "127.0.0.1,192.168.1.100"

# Generate client certificate
muti-metroo cert client \
  --cn "client-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key

# View certificate details
muti-metroo cert info ./certs/agent-1.crt
```

### 17.4 Certificate Details

**Algorithm:** ECDSA P-256

**Output Files:**

- `{name}.crt` - PEM-encoded certificate (mode 0644)
- `{name}.key` - PEM-encoded private key (mode 0600)

**Fingerprint Format:** `sha256:<hex>`

**Key Usages:**

- CA: Certificate signing, CRL signing
- Server: Digital signature, key encipherment, server auth
- Client: Digital signature, client auth
- Peer: Digital signature, key encipherment, server auth, client auth

---

## 18. Project Structure

### 18.1 Directory Layout

```
muti-metroo/
├── cmd/
│   └── muti-metroo/
│       └── main.go                 # CLI entrypoint and all commands
│
├── internal/
│   ├── agent/
│   │   ├── agent.go                # Main agent orchestration
│   │   └── agent_test.go           # Agent tests
│   │
│   ├── config/
│   │   ├── config.go               # Configuration parsing and validation
│   │   └── config_test.go          # Configuration tests
│   │
│   ├── identity/
│   │   ├── identity.go             # AgentID generation/storage
│   │   ├── keypair.go              # X25519 keypair for E2E encryption
│   │   ├── identity_test.go        # Identity tests
│   │   └── keypair_test.go         # Keypair tests
│   │
│   ├── crypto/
│   │   ├── crypto.go               # E2E encryption: X25519 + ChaCha20-Poly1305
│   │   ├── sealed.go               # Sealed box for management key encryption
│   │   ├── crypto_test.go          # Crypto tests
│   │   └── sealed_test.go          # Sealed box tests
│   │
│   ├── transport/
│   │   ├── transport.go            # Transport interface
│   │   ├── quic.go                 # QUIC implementation
│   │   ├── h2.go                   # HTTP/2 implementation
│   │   ├── ws.go                   # WebSocket implementation
│   │   ├── tls.go                  # TLS helpers
│   │   ├── transport_test.go       # Transport tests
│   │   ├── h2_test.go              # HTTP/2 tests
│   │   └── ws_test.go              # WebSocket tests
│   │
│   ├── peer/
│   │   ├── manager.go              # Peer lifecycle management
│   │   ├── connection.go           # Single peer connection
│   │   ├── handshake.go            # PEER_HELLO handling
│   │   ├── reconnect.go            # Reconnection logic
│   │   ├── peer_test.go            # Peer tests
│   │   └── handshake_test.go       # Handshake tests
│   │
│   ├── protocol/
│   │   ├── frame.go                # Frame encode/decode
│   │   ├── types.go                # Frame types, flags, error codes
│   │   └── protocol_test.go        # Protocol tests
│   │
│   ├── stream/
│   │   ├── manager.go              # Stream lifecycle and forward table
│   │   └── stream_test.go          # Stream tests
│   │
│   ├── routing/
│   │   ├── table.go                # Route table
│   │   ├── manager.go              # Route management
│   │   └── routing_test.go         # Routing tests
│   │
│   ├── flood/
│   │   ├── flood.go                # Flood protocol (advertise/withdraw)
│   │   └── flood_test.go           # Flood tests
│   │
│   ├── forward/
│   │   ├── forward.go              # Endpoint struct, ForwardDialer interface
│   │   ├── handler.go              # Exit point handler for port forwarding
│   │   ├── listener.go             # TCP listener for incoming connections
│   │   ├── handler_test.go         # Handler unit tests
│   │   └── listener_test.go        # Listener unit tests
│   │
│   ├── socks5/
│   │   ├── server.go               # SOCKS5 server
│   │   ├── handler.go              # SOCKS5 command handler
│   │   ├── auth.go                 # Authentication
│   │   ├── ws_listener.go          # WebSocket SOCKS5 listener
│   │   ├── conn_tracker.go         # Generic connection tracker
│   │   └── socks5_test.go          # SOCKS5 tests
│   │
│   ├── exit/
│   │   ├── handler.go              # Exit handler
│   │   ├── dns.go                  # DNS resolution
│   │   └── exit_test.go            # Exit tests
│   │
│   ├── embed/
│   │   ├── embed.go                # Binary config embedding (XOR obfuscation)
│   │   └── embed_test.go           # Embed tests
│   │
│   ├── certutil/
│   │   ├── certutil.go             # Certificate generation and management
│   │   └── certutil_test.go        # Certificate tests
│   │
│   ├── health/
│   │   ├── server.go               # Health check HTTP server
│   │   └── server_test.go          # Health server tests
│   │
│   ├── udp/
│   │   ├── handler.go              # UDP relay handler (SOCKS5 UDP ASSOCIATE)
│   │   ├── association.go          # UDP association lifecycle management
│   │   ├── config.go               # UDP configuration
│   │   └── doc.go                  # Package documentation
│   │
│   ├── icmp/
│   │   ├── handler.go              # ICMP echo (ping) handler at exit node
│   │   ├── socket.go               # Platform-specific ICMP socket operations
│   │   ├── session.go              # ICMP session state management
│   │   ├── config.go               # ICMP CIDR validation and configuration
│   │   ├── config_test.go          # Config tests
│   │   └── session_test.go         # Session tests
│   │
│   ├── probe/
│   │   └── probe.go                # Connectivity testing for listeners
│   │
│   ├── wizard/
│   │   ├── wizard.go               # Setup wizard implementation
│   │   └── wizard_test.go          # Wizard tests
│   │
│   ├── service/
│   │   ├── service.go              # Service management interface
│   │   ├── service_linux.go        # Linux systemd implementation
│   │   ├── service_darwin.go       # macOS launchd implementation
│   │   ├── service_windows.go      # Windows service implementation
│   │   ├── service_other.go        # Stub for unsupported platforms
│   │   └── service_test.go         # Service tests
│   │
│   ├── shell/
│   │   ├── shell.go                # Remote shell executor
│   │   ├── handler.go              # Shell request/response handling
│   │   ├── pty_unix.go             # PTY allocation for Unix platforms
│   │   ├── pty_windows.go          # ConPTY for Windows
│   │   ├── streaming.go            # Non-PTY streaming command execution
│   │   └── shell_test.go           # Shell tests
│   │
│   ├── sleep/
│   │   ├── sleep.go                # Sleep manager state machine, persistence
│   │   ├── queue.go                # State queue for sleeping peers
│   │   ├── window.go               # Deterministic listening window calculator
│   │   ├── sleep_test.go           # Sleep tests
│   │   └── window_test.go          # Window calculator tests
│   │
│   ├── filetransfer/
│   │   ├── stream.go               # Stream-based file transfer protocol
│   │   ├── tar.go                  # Directory tar/untar with gzip compression
│   │   ├── stream_test.go          # Stream transfer tests
│   │   └── tar_test.go             # Tar archive tests
│   │
│   ├── sysinfo/
│   │   └── sysinfo.go              # System info and shell detection for node advertisements
│   │
│   ├── logging/
│   │   ├── logging.go              # Structured logging utilities
│   │   └── logging_test.go         # Logging tests
│   │
│   ├── recovery/
│   │   ├── recovery.go             # Panic recovery utilities
│   │   └── recovery_test.go        # Recovery tests
│   │
│   ├── chaos/
│   │   ├── chaos.go                # Fault injection for testing
│   │   └── chaos_test.go           # Chaos testing tests
│   │
│   ├── loadtest/
│   │   ├── loadtest.go             # Load testing utilities
│   │   └── loadtest_test.go        # Load test tests
│   │
│   └── integration/
│       ├── chain_test.go           # Multi-agent chain tests
│       ├── e2e_stream_test.go      # End-to-end stream tests
│       └── ...                     # Additional integration tests
│
├── configs/
│   └── example.yaml                # Example configuration
│
├── docs/
│   └── RUNBOOK.md                  # Operational runbook
│
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── Dockerfile.test
└── README.md
```

### 18.2 Dependencies

| Package                             | Purpose                               |
| ----------------------------------- | ------------------------------------- |
| `github.com/quic-go/quic-go`        | QUIC transport                        |
| `golang.org/x/net/http2`            | HTTP/2 transport                      |
| `nhooyr.io/websocket`               | WebSocket transport                   |
| `github.com/refraction-networking/utls` | TLS fingerprint customization (JA3/JA4 blending) |
| `gopkg.in/yaml.v3`                  | Configuration parsing                 |
| `github.com/spf13/cobra`            | CLI framework                         |
| `log/slog`                          | Structured logging (stdlib)           |
| `github.com/charmbracelet/huh`      | Interactive setup wizard TUI          |
| `github.com/charmbracelet/lipgloss` | Terminal styling                      |

---

## 19. Implementation Notes

### 19.1 Critical Settings

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CRITICAL SETTINGS                                    │
│                                                                             │
│  TCP_NODELAY (Nagle's algorithm):                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  MUST disable Nagle's algorithm on all TCP connections.             │    │
│  │  Without this, small packets (SSH keystrokes) are delayed.          │    │
│  │                                                                     │    │
│  │  Go: conn.(*net.TCPConn).SetNoDelay(true)                           │    │
│  │                                                                     │    │
│  │  Apply to:                                                          │    │
│  │  • HTTP/2 transport connections                                     │    │
│  │  • WebSocket transport connections                                  │    │
│  │  • Exit handler TCP connections                                     │    │
│  │  • SOCKS5 client connections                                        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Buffer sizes:                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Default: 256 KB per stream                                         │    │
│  │  Suitable for video streaming                                       │    │
│  │  Configurable for memory-constrained environments                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Frame size:                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Maximum: 16 KB payload                                             │    │
│  │  Ensures fairness between streams                                   │    │
│  │  Prevents one stream from blocking others                           │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 19.2 Goroutine Management

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       GOROUTINE MANAGEMENT                                  │
│                                                                             │
│  Per peer connection:                                                       │
│  • 1 reader goroutine (reads frames, dispatches)                            │
│  • 1 writer goroutine (serializes outgoing frames)                          │
│  • 1 keepalive goroutine (periodic checks)                                  │
│                                                                             │
│  Per stream (at endpoints):                                                 │
│  • 2 goroutines for bidirectional relay                                     │
│                                                                             │
│  Leak prevention:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  • Use context.Context for cancellation                             │    │
│  │  • Set read/write deadlines                                         │    │
│  │  • Close underlying connections on shutdown                         │    │
│  │  • Use recovery.RecoverWithLog for panic handling                   │    │
│  │  • Log goroutine count periodically (debug mode)                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 19.3 Error Handling

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ERROR HANDLING                                      │
│                                                                             │
│  Connection errors:                                                         │
│  • Log error with peer ID                                                   │
│  • Clean up streams (send STREAM_RESET)                                     │
│  • Remove routes via this peer                                              │
│  • Trigger reconnection                                                     │
│                                                                             │
│  Stream errors:                                                             │
│  • Log error with stream ID                                                 │
│  • Send STREAM_RESET to both ends                                           │
│  • Remove from forward table                                                │
│  • Close local connections if any                                           │
│                                                                             │
│  Flood propagation errors:                                                  │
│  • Log at debug level (non-fatal)                                           │
│  • Continue sending to other peers                                          │
│  • Do not fail the entire flood operation                                   │
│                                                                             │
│  Configuration errors:                                                      │
│  • Validate on startup                                                      │
│  • Fail fast with clear error message                                       │
│                                                                             │
│  Resource exhaustion:                                                       │
│  • Reject new streams with RESOURCE_LIMIT error                             │
│  • Log warning                                                              │
│  • Don't kill existing streams                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 20. Testing Strategy

### 20.1 Test Levels

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          TEST LEVELS                                        │
│                                                                             │
│  Unit Tests:                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  • Frame encode/decode                                              │    │
│  │  • Route table operations (insert, lookup, expire)                  │    │
│  │  • Stream state machine transitions                                 │    │
│  │  • CIDR matching                                                    │    │
│  │  • Configuration parsing and validation                             │    │
│  │  • Certificate generation                                           │    │
│  │  • Wizard config building                                           │    │
│  │  • Service unit generation                                          │    │
│  │  • Handshake failure scenarios                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  Integration Tests:                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  • 2-agent chain (ingress → exit)                                   │    │
│  │  • 3-agent chain (ingress → transit → exit)                         │    │
│  │  • Each transport type (QUIC, HTTP/2, WebSocket)                    │    │
│  │  • Reconnection behavior                                            │    │
│  │  • Route propagation                                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  End-to-End Tests:                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  • SSH session through 3-agent chain                                │    │
│  │  • HTTP request/response                                            │    │
│  │  • Large file transfer                                              │    │
│  │  • Concurrent streams (SSH + video simulation)                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 20.2 Running Tests

```bash
# Run all tests with race detection
make test

# Run with coverage report
make test-coverage

# Run specific package tests
go test -v ./internal/config/...

# Run single test
go test -v -run TestHandshake_Success ./internal/peer/

# Run tests in Docker
docker build -f Dockerfile.test -t muti-metroo-test .
docker run --rm muti-metroo-test
```

---

## Appendix A: Quick Reference

### Frame Types

| Code | Name                | Description            |
| ---- | ------------------- | ---------------------- |
| 0x01 | STREAM_OPEN         | Open virtual stream    |
| 0x02 | STREAM_OPEN_ACK     | Stream opened          |
| 0x03 | STREAM_OPEN_ERR     | Stream failed          |
| 0x04 | STREAM_DATA         | Payload data           |
| 0x05 | STREAM_CLOSE        | Graceful close         |
| 0x06 | STREAM_RESET        | Abort stream           |
| 0x10 | ROUTE_ADVERTISE     | Announce routes        |
| 0x11 | ROUTE_WITHDRAW      | Remove routes          |
| 0x12 | NODE_INFO_ADVERTISE | Announce node metadata |
| 0x20 | PEER_HELLO          | Handshake              |
| 0x21 | PEER_HELLO_ACK      | Handshake response     |
| 0x22 | KEEPALIVE           | Liveness probe         |
| 0x23 | KEEPALIVE_ACK       | Liveness response      |
| 0x24 | CONTROL_REQUEST     | Request status/RPC     |
| 0x25 | CONTROL_RESPONSE    | Response with data     |
| 0x30 | UDP_OPEN            | Request UDP association |
| 0x31 | UDP_OPEN_ACK        | Association established |
| 0x32 | UDP_OPEN_ERR        | Association failed     |
| 0x33 | UDP_DATAGRAM        | UDP datagram payload   |
| 0x34 | UDP_CLOSE           | Close association      |
| 0x50 | SLEEP_COMMAND       | Mesh-wide sleep        |
| 0x51 | WAKE_COMMAND        | Mesh-wide wake         |
| 0x52 | QUEUED_STATE        | Queued state for peer  |

### Error Codes

| Code | Name                 | Description                      |
| ---- | -------------------- | -------------------------------- |
| 1    | NO_ROUTE             | No route to destination          |
| 2    | CONNECTION_REFUSED   | Target refused connection        |
| 3    | CONNECTION_TIMEOUT   | Connection attempt timed out     |
| 4    | TTL_EXCEEDED         | TTL reached zero                 |
| 5    | HOST_UNREACHABLE     | Cannot reach target host         |
| 6    | NETWORK_UNREACHABLE  | Cannot reach target network      |
| 7    | DNS_ERROR            | Domain name resolution failed    |
| 8    | EXIT_DISABLED        | Exit functionality not enabled   |
| 9    | RESOURCE_LIMIT       | Too many streams                 |
| 10   | CONNECTION_LIMIT     | Connection limit exceeded        |
| 11   | NOT_ALLOWED          | Operation not permitted          |
| 12   | FILE_TRANSFER_DENIED | File transfer not allowed        |
| 13   | AUTH_REQUIRED        | Authentication required          |
| 14   | PATH_NOT_ALLOWED     | Path not in allowed list         |
| 15   | FILE_TOO_LARGE       | File exceeds size limit          |
| 16   | FILE_NOT_FOUND       | File does not exist              |
| 17   | WRITE_FAILED         | Write operation failed           |
| 18   | GENERAL_FAILURE      | General error                    |
| 19   | RESUME_FAILED        | Resume not possible              |
| 20   | SHELL_DISABLED       | Shell feature is disabled        |
| 21   | SHELL_AUTH_FAILED    | Shell authentication failed      |
| 22   | PTY_FAILED           | PTY allocation failed            |
| 23   | COMMAND_NOT_ALLOWED  | Command not in whitelist         |
| 30   | UDP_DISABLED         | UDP relay is disabled            |
| 31   | UDP_PORT_NOT_ALLOWED | UDP port not in whitelist        |
| 40   | FORWARD_NOT_FOUND    | Port forward key not configured  |

### Default Timing

| Parameter                | Default |
| ------------------------ | ------- |
| Keepalive idle threshold | 5m      |
| Keepalive timeout        | 90s     |
| Route TTL                | 5m      |
| Advertise interval       | 2m      |
| Reconnect initial delay  | 1s      |
| Reconnect max delay      | 60s     |
| Stream open timeout      | 30s     |
| Handshake timeout        | 10s     |

### CLI Commands

| Command             | Description                            |
| ------------------- | -------------------------------------- |
| `setup`             | Interactive configuration wizard       |
| `init`              | Initialize agent identity              |
| `run`               | Start the agent                        |
| `status`            | Show agent status                      |
| `peers`             | List connected peers                   |
| `routes`            | Show routing table                     |
| `probe`             | Test connectivity to a listener        |
| `shell`             | Execute command on remote agent        |
| `upload`            | Upload file to remote agent            |
| `download`          | Download file from remote agent        |
| `cert ca`           | Generate CA certificate                |
| `cert agent`        | Generate agent certificate             |
| `cert client`       | Generate client certificate            |
| `cert info`         | Display certificate details            |
| `hash`              | Generate bcrypt password hash          |
| `management-key`    | Generate mesh topology encryption keys |
| `service install`   | Install as system service              |
| `service uninstall` | Remove system service                  |
| `service status`    | Check service status                   |
| `sleep`             | Put mesh into sleep mode               |
| `wake`              | Wake mesh from sleep mode              |
| `sleep-status`      | Check sleep mode status                |

---

## Appendix B: Mutiauk - Optional TUN Interface

Mutiauk is an optional companion tool that provides transparent Layer 3 traffic interception, forwarding traffic through Muti Metroo's SOCKS5 proxy.

### Overview

While Muti Metroo requires applications to be SOCKS5-aware, Mutiauk enables any application to use the mesh network without modification by:

1. Creating a TUN network interface
2. Intercepting L3 (IP) traffic destined for configured routes
3. Forwarding TCP and UDP connections through SOCKS5
4. Returning responses back through the TUN interface

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MUTIAUK DATA FLOW                                   │
│                                                                             │
│  Application → TUN Device → gVisor Stack → TCP/UDP Forwarder → SOCKS5 →     │
│                                                               Muti Metroo → │
│                                                               Exit Agent →  │
│                                                               Destination   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Platform Requirements

| Aspect           | Requirement                   |
| ---------------- | ----------------------------- |
| **Platform**     | Linux only (TUN interface)    |
| **Privileges**   | Root required (CAP_NET_ADMIN) |
| **Dependencies** | None (static binary)          |

**Note:** Unlike Muti Metroo agents which run unprivileged, Mutiauk requires root to create and manage the TUN interface.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MUTIAUK ARCHITECTURE                                │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                          DAEMON                                       │  │
│  │                                                                       │  │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │  │
│  │  │  TUN Device    │  │  gVisor Stack  │  │  NAT Table     │           │  │
│  │  │                │  │                │  │                │           │  │
│  │  │ • /dev/net/tun │  │ • TCP forwarder│  │ • Connection   │           │  │
│  │  │ • TUNSETIFF    │  │ • UDP forwarder│  │   tracking     │           │  │
│  │  │ • IP packets   │  │ • gonet        │  │ • TTL expiry   │           │  │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │  │
│  │                                                                       │  │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │  │
│  │  │  Route Manager │  │  SOCKS5 Client │  │  Config Watch  │           │  │
│  │  │                │  │                │  │                │           │  │
│  │  │ • netlink      │  │ • CONNECT      │  │ • fsnotify     │           │  │
│  │  │ • CIDR routes  │  │ • UDP ASSOCIATE│  │ • hot reload   │           │  │
│  │  │ • conflict det │  │ • auth         │  │ • SIGHUP       │           │  │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Package Structure

```
mutiauk/
├── cmd/mutiauk/
│   └── main.go              # CLI entrypoint
├── internal/
│   ├── daemon/              # Orchestrator (server.go)
│   ├── tun/                 # TUN device creation
│   ├── stack/               # gVisor TCP/IP stack wrapper
│   ├── proxy/               # TCP/UDP forwarders
│   ├── socks5/              # SOCKS5 client
│   ├── nat/                 # Connection tracking
│   ├── route/               # Linux route management
│   ├── config/              # YAML configuration
│   ├── cli/                 # CLI commands
│   ├── wizard/              # Interactive setup wizard
│   └── service/             # Systemd service management
└── configs/
    └── mutiauk.example.yaml
```

### CLI Commands

| Command             | Description                      |
| ------------------- | -------------------------------- |
| `setup`             | Interactive configuration wizard |
| `daemon start`      | Start the daemon                 |
| `daemon stop`       | Stop the daemon                  |
| `daemon status`     | Check daemon status              |
| `daemon reload`     | Reload configuration             |
| `route list`        | List active routes               |
| `route add`         | Add a route                      |
| `route remove`      | Remove a route                   |
| `status`            | Show comprehensive status        |
| `service install`   | Install systemd service          |
| `service uninstall` | Remove systemd service           |

### Configuration

```yaml
# /etc/mutiauk/config.yaml
daemon:
  pid_file: /var/run/mutiauk.pid
  socket_path: /var/run/mutiauk.sock

tun:
  name: tun0
  mtu: 1400
  address: 10.200.200.1/24

socks5:
  server: 127.0.0.1:1080 # Muti Metroo SOCKS5 address
  username: ""
  password: ""
  timeout: 30s

routes:
  - destination: 10.0.0.0/8
    comment: "Internal network"
    enabled: true

nat:
  table_size: 65536
  tcp_timeout: 1h
  udp_timeout: 5m
```

### Relationship with Muti Metroo

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     MUTIAUK + MUTI METROO DEPLOYMENT                        │
│                                                                             │
│  ┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐     │
│  │   Application    │     │     Mutiauk      │     │   Muti Metroo    │     │
│  │   (any app)      │     │  (Linux, root)   │     │   (userspace)    │     │
│  │                  │     │                  │     │                  │     │
│  │  No SOCKS5       │     │  TUN interface   │     │  SOCKS5 ingress  │     │
│  │  configuration   │────►│  IP → SOCKS5     │────►│  Mesh routing    │     │
│  │  required        │     │  TCP + UDP       │     │  E2E encryption  │     │
│  │                  │     │                  │     │                  │     │
│  └──────────────────┘     └──────────────────┘     └──────────────────┘     │
│                                                              │              │
│                                                              ▼              │
│                                                    ┌──────────────────┐     │
│                                                    │   Exit Agent     │     │
│                                                    │                  │     │
│                                                    │  Real TCP/UDP    │     │
│                                                    │  connections     │     │
│                                                    └──────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Documentation

Mutiauk documentation is available at: https://mutimetroo.com/mutiauk

Source code location: `../Mutiauk/` (sibling directory to Muti Metroo)

---

_End of Document_
