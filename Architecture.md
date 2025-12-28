# Mesh Agent Network Architecture

**Version:** 1.0
**Date:** December 2024

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [System Overview](#2-system-overview)
3. [Core Components](#3-core-components)
4. [Identity and Addressing](#4-identity-and-addressing)
5. [Transport Layer](#5-transport-layer)
6. [Frame Protocol](#6-frame-protocol)
7. [Stream Management](#7-stream-management)
8. [Routing System](#8-routing-system)
9. [Flood Protocol](#9-flood-protocol)
10. [Peer Connection Management](#10-peer-connection-management)
11. [SOCKS5 Server](#11-socks5-server)
12. [Data Plane](#12-data-plane)
13. [Configuration](#13-configuration)
14. [Security](#14-security)
15. [Project Structure](#15-project-structure)
16. [Implementation Notes](#16-implementation-notes)
17. [Testing Strategy](#17-testing-strategy)

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

### 1.3 Target Scale

This architecture targets small to medium deployments:

- **Agents:** Up to 20
- **Concurrent users:** Up to 10
- **Traffic types:** Interactive (SSH), streaming (video), bulk transfers
- **Typical topology:** Chains of 2-5 agents with occasional branching

The design prioritizes simplicity and correctness over extreme scalability.

### 1.4 Implementation Language

The reference implementation will be written in **Go**, leveraging its excellent concurrency primitives and networking libraries.

---

## 2. System Overview

### 2.1 Network Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MESH NETWORK                                   │
│                                                                             │
│  ┌─────────┐      QUIC       ┌─────────┐     HTTP/2     ┌─────────┐        │
│  │ Agent 1 │◄───────────────►│ Agent 2 │◄──────────────►│ Agent 3 │        │
│  │         │    (UDP/4433)   │         │   (TCP/8443)   │         │        │
│  │ Entry   │                 │ Transit │                │  Exit   │        │
│  │ SOCKS5  │                 │         │                │ 10.0.0.0/8       │
│  │ :1080   │                 │         │                │         │        │
│  └────▲────┘                 └─────────┘                └────┬────┘        │
│       │                                                      │              │
│       │ Client                                          Real │ TCP          │
│       │ Application                                          │              │
│  ┌────┴────┐                                           ┌─────▼─────┐       │
│  │  curl   │                                           │  Target   │       │
│  │ Browser │                                           │  Server   │       │
│  │  SSH    │                                           │ 10.5.3.100│       │
│  └─────────┘                                           └───────────┘       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Agent Roles

An agent can serve one or more roles simultaneously:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AGENT ROLES                                    │
│                                                                             │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐       │
│  │      INGRESS      │  │      TRANSIT      │  │       EXIT        │       │
│  │                   │  │                   │  │                   │       │
│  │ • SOCKS5 listener │  │ • Forward streams │  │ • Open real TCP   │       │
│  │ • Initiates       │  │ • Route flooding  │  │ • Advertise CIDRs │       │
│  │   virtual streams │  │ • No local I/O    │  │ • DNS resolution  │       │
│  │ • Route lookup    │  │                   │  │                   │       │
│  └───────────────────┘  └───────────────────┘  └───────────────────┘       │
│                                                                             │
│  Deployment Examples:                                                       │
│  • Home laptop:     Ingress only (SOCKS5 for browser)                      │
│  • Cloud relay:     Transit only (forward between networks)                │
│  • Office gateway:  Exit only (access to internal network)                 │
│  • VPN endpoint:    Ingress + Exit (full VPN replacement)                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Traffic Flow Example

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         END-TO-END TRAFFIC FLOW                             │
│                                                                             │
│  1. User runs: ssh -o ProxyCommand='nc -x localhost:1080 %h %p' server     │
│                                                                             │
│  2. SSH connects to SOCKS5 proxy on Agent1 (localhost:1080)                │
│                                                                             │
│  3. SOCKS5 receives: CONNECT server.internal:22                            │
│                                                                             │
│  4. Agent1 looks up route for "server.internal"                            │
│     Result: Exit=Agent3, Path=[Agent2, Agent3]                             │
│                                                                             │
│  5. Agent1 sends STREAM_OPEN to Agent2:                                    │
│     - Destination: server.internal:22                                      │
│     - RemainingPath: [Agent3]                                              │
│                                                                             │
│  6. Agent2 forwards STREAM_OPEN to Agent3:                                 │
│     - RemainingPath: [] (empty = I am exit)                                │
│                                                                             │
│  7. Agent3 resolves "server.internal" via local DNS                        │
│     Opens TCP connection to server.internal:22                             │
│     Sends STREAM_OPEN_ACK back through chain                               │
│                                                                             │
│  8. Agent1 receives ACK, sends SOCKS5 success to SSH                       │
│                                                                             │
│  9. SSH session data flows bidirectionally through the chain               │
│     User ↔ Agent1 ↔ Agent2 ↔ Agent3 ↔ SSH Server                          │
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
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                          CONTROL PLANE                                 │ │
│  │                                                                        │ │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │ │
│  │  │  Route Table   │  │ Flood Protocol │  │  Peer Manager  │           │ │
│  │  │                │  │                │  │                │           │ │
│  │  │ • CIDR entries │  │ • Advertise    │  │ • Connections  │           │ │
│  │  │ • LPM lookup   │  │ • Withdraw     │  │ • Reconnection │           │ │
│  │  │ • Path cache   │  │ • Loop prevent │  │ • Handshake    │           │ │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │ │
│  │                                                                        │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                           DATA PLANE                                   │ │
│  │                                                                        │ │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │ │
│  │  │ Stream Manager │  │ Forward Table  │  │  Exit Handler  │           │ │
│  │  │                │  │                │  │                │           │ │
│  │  │ • Lifecycle    │  │ • Stream map   │  │ • TCP connect  │           │ │
│  │  │ • Half-close   │  │ • Relay data   │  │ • DNS resolve  │           │ │
│  │  │ • Fairness     │  │ • Cleanup      │  │ • Error handle │           │ │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │ │
│  │                                                                        │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                        TRANSPORT LAYER                                 │ │
│  │                                                                        │ │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐           │ │
│  │  │     QUIC       │  │    HTTP/2      │  │   WebSocket    │           │ │
│  │  │    (UDP)       │  │   Streaming    │  │   HTTP/1.1     │           │ │
│  │  │                │  │                │  │                │           │ │
│  │  │ • Native mux   │  │ • Single POST  │  │ • Proxy compat │           │ │
│  │  │ • 0-RTT        │  │ • Bidi stream  │  │ • Upgrade      │           │ │
│  │  │ • Migration    │  │ • Port 443     │  │ • Binary frame │           │ │
│  │  └────────────────┘  └────────────────┘  └────────────────┘           │ │
│  │                                                                        │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         INGRESS LAYER                                  │ │
│  │                                                                        │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │ │
│  │  │                       SOCKS5 Server                             │  │ │
│  │  │  • CONNECT command  • IPv4/IPv6/Domain  • Optional auth        │  │ │
│  │  └─────────────────────────────────────────────────────────────────┘  │ │
│  │                                                                        │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
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

---

## 4. Identity and Addressing

### 4.1 Agent Identity

Each agent has a persistent unique identifier:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             AGENT IDENTITY                                  │
│                                                                             │
│  AgentID: 16 bytes (128 bits)                                              │
│                                                                             │
│  • Generated randomly on first run using crypto/rand                       │
│  • Persisted to data directory                                             │
│  • Displayed as hex string: "a3f8c2d1e5b94a7c8d2e1f0a3b5c7d9e"            │
│  • Short form for logs: "a3f8c2d1" (first 8 hex chars)                    │
│                                                                             │
│  Storage: {data_dir}/agent_id                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Stream Identification

Streams are identified by a combination of peer connection and stream ID:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          STREAM IDENTIFICATION                              │
│                                                                             │
│  Stream IDs are scoped to a peer connection.                               │
│                                                                             │
│  Allocation scheme (prevents collisions):                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Connection initiator (dialer):   ODD  stream IDs (1, 3, 5, 7...)  │   │
│  │  Connection acceptor (listener):  EVEN stream IDs (2, 4, 6, 8...)  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Each side maintains its own counter, incrementing by 2.                   │
│  StreamID 0 is reserved for the control channel.                           │
│                                                                             │
│  Global stream reference (for logging/debugging):                          │
│    {PeerID}:{StreamID}                                                     │
│    Example: "a3f8c2d1:42"                                                  │
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
│    QUIC:       quic://192.168.1.50:4433                                    │
│                quic://[2001:db8::1]:4433                                   │
│                                                                             │
│    HTTP/2:     h2://192.168.1.50:8443/mesh                                 │
│                h2://relay.example.com:443/mesh                             │
│                                                                             │
│    WebSocket:  ws://192.168.1.50:8080/mesh                                 │
│                wss://relay.example.com:443/mesh                            │
│                                                                             │
│  Destination addresses (STREAM_OPEN):                                       │
│                                                                             │
│    Type 0x01:  IPv4      4 bytes     (e.g., 10.5.3.100)                   │
│    Type 0x04:  IPv6      16 bytes    (e.g., 2001:db8::1)                  │
│    Type 0x03:  Domain    1+N bytes   (length-prefixed string)             │
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
│  Implementation: github.com/quic-go/quic-go                                │
│                                                                             │
│  Characteristics:                                                           │
│  • Each virtual stream maps to native QUIC stream                          │
│  • No head-of-line blocking between streams                                │
│  • Built-in TLS 1.3 encryption                                             │
│  • Connection migration (survives IP changes)                              │
│  • 0-RTT session resumption                                                │
│                                                                             │
│  Configuration:                                                             │
│  • Max streams: 10,000                                                     │
│  • Idle timeout: 60s                                                       │
│  • Keepalive: 30s                                                          │
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
│  Implementation: golang.org/x/net/http2                                    │
│                                                                             │
│  Characteristics:                                                           │
│  • Single HTTP/2 POST request with streaming body                          │
│  • Response body also streams (bidirectional once established)             │
│  • Our frame protocol provides multiplexing                                │
│  • Works on port 443, traverses most firewalls                             │
│                                                                             │
│  Key implementation details:                                                │
│  • Use io.Pipe for request body streaming                                  │
│  • Call http.Flusher.Flush() to disable buffering                          │
│  • Set Content-Type: application/octet-stream                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.6 WebSocket HTTP/1.1 Transport

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      WEBSOCKET HTTP/1.1 TRANSPORT                           │
│                                                                             │
│  ┌───────────────┐    ┌───────────┐    ┌───────────────┐                   │
│  │    Agent A    │────│   HTTP    │────│    Agent B    │                   │
│  │   (client)    │    │   Proxy   │    │   (server)    │                   │
│  └───────┬───────┘    └─────┬─────┘    └───────┬───────┘                   │
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
│  Implementation: nhooyr.io/websocket                                       │
│                                                                             │
│  Characteristics:                                                           │
│  • Maximum compatibility with corporate proxies                            │
│  • HTTP CONNECT tunneling for proxy traversal                              │
│  • After upgrade, fully symmetric bidirectional                            │
│  • Binary frames for efficiency                                            │
│  • Our frame protocol provides multiplexing                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.7 Multiplexing Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MULTIPLEXING STRATEGY                               │
│                                                                             │
│  QUIC:                                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Each virtual stream = dedicated QUIC stream                        │   │
│  │  No additional framing needed for multiplexing                      │   │
│  │  Frames still used for control messages and stream metadata         │   │
│  │  Benefit: Native per-stream flow control, no HOL blocking          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  HTTP/2 and WebSocket:                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Single transport stream carries all virtual streams                │   │
│  │  Our frame protocol provides multiplexing via StreamID              │   │
│  │  Writer uses round-robin for fairness between streams               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Fairness (HTTP/2 and WebSocket):                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Problem: Video stream could starve SSH stream                      │   │
│  │                                                                     │   │
│  │  Solution:                                                          │   │
│  │  • Maximum frame payload: 16 KB                                     │   │
│  │  • Writer maintains queue per stream                                │   │
│  │  • Round-robin between streams with pending data                    │   │
│  │  • No stream can send more than one frame before others get a turn  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
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
│    0       1       2       3       4       5       6       7       8       │
│   ┌───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┐       │
│   │ Type  │ Flags │            Length             │    StreamID   │       │
│   │  1B   │  1B   │              4B               │       8B      │       │
│   ├───────┴───────┴───────────────────────────────┴───────────────┤       │
│   │                                                               │       │
│   │                          Payload                              │       │
│   │                       (Length bytes)                          │       │
│   │                                                               │       │
│   └───────────────────────────────────────────────────────────────┘       │
│                                                                             │
│   Header size: 14 bytes                                                    │
│   Maximum payload: 16,384 bytes (16 KB)                                    │
│   Maximum frame size: 16,398 bytes                                         │
│                                                                             │
│   Byte order: Big-endian (network byte order)                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Frame Types

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             FRAME TYPES                                     │
│                                                                             │
│  Stream Frames:                                                             │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐ │
│  │ Type │ Name               │ Direction   │ Purpose                     │ │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤ │
│  │ 0x01 │ STREAM_OPEN        │ Forward     │ Request to open stream      │ │
│  │ 0x02 │ STREAM_OPEN_ACK    │ Backward    │ Stream opened successfully  │ │
│  │ 0x03 │ STREAM_OPEN_ERR    │ Backward    │ Stream open failed          │ │
│  │ 0x04 │ STREAM_DATA        │ Both        │ Payload data                │ │
│  │ 0x05 │ STREAM_CLOSE       │ Both        │ Graceful close (half/full)  │ │
│  │ 0x06 │ STREAM_RESET       │ Both        │ Abort stream with error     │ │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘ │
│                                                                             │
│  Routing Frames:                                                            │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐ │
│  │ Type │ Name               │ Direction   │ Purpose                     │ │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤ │
│  │ 0x10 │ ROUTE_ADVERTISE    │ Flood       │ Announce CIDR routes        │ │
│  │ 0x11 │ ROUTE_WITHDRAW     │ Flood       │ Remove CIDR routes          │ │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘ │
│                                                                             │
│  Control Frames:                                                            │
│  ┌──────┬────────────────────┬─────────────┬─────────────────────────────┐ │
│  │ Type │ Name               │ Direction   │ Purpose                     │ │
│  ├──────┼────────────────────┼─────────────┼─────────────────────────────┤ │
│  │ 0x20 │ PEER_HELLO         │ Initiator   │ Initial handshake           │ │
│  │ 0x21 │ PEER_HELLO_ACK     │ Acceptor    │ Handshake response          │ │
│  │ 0x22 │ KEEPALIVE          │ Either      │ Liveness probe              │ │
│  │ 0x23 │ KEEPALIVE_ACK      │ Either      │ Liveness response           │ │
│  └──────┴────────────────────┴─────────────┴─────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.3 Frame Flags

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              FRAME FLAGS                                    │
│                                                                             │
│   Bit   │ Name          │ Applicable To      │ Meaning                     │
│  ───────┼───────────────┼────────────────────┼─────────────────────────────│
│    0    │ FIN_WRITE     │ STREAM_CLOSE       │ Sender is done writing      │
│    1    │ FIN_READ      │ STREAM_CLOSE       │ Sender is done reading      │
│    2    │ (reserved)    │                    │                             │
│    3    │ (reserved)    │                    │                             │
│   4-7   │ (reserved)    │                    │                             │
│                                                                             │
│   STREAM_CLOSE flag combinations:                                           │
│   0x01 = FIN_WRITE only    → Half-close (done sending)                     │
│   0x02 = FIN_READ only     → Half-close (done receiving)                   │
│   0x03 = FIN_WRITE|READ    → Full close                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.4 Payload Definitions

#### PEER_HELLO (0x20)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PEER_HELLO                                     │
│                                                                             │
│   Sent by connection initiator after transport is established.             │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ Version         │ 2      │ Protocol version (currently 1)          │  │
│   │ AgentID         │ 16     │ Sender's agent ID                       │  │
│   │ Timestamp       │ 8      │ Unix timestamp (seconds)                │  │
│   │ CapabilitiesLen │ 1      │ Number of capabilities                  │  │
│   │ Capabilities    │ varies │ List of capability strings              │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Capability string format: 1-byte length + UTF-8 string                   │
│   Known capabilities: "exit", "socks5"                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### PEER_HELLO_ACK (0x21)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            PEER_HELLO_ACK                                   │
│                                                                             │
│   Sent by connection acceptor in response to PEER_HELLO.                   │
│   Same format as PEER_HELLO.                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN (0x01)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             STREAM_OPEN                                     │
│                                                                             │
│   Request to open a virtual stream to a destination.                       │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ RequestID       │ 8      │ Unique ID for correlating ACK/ERR       │  │
│   │ AddressType     │ 1      │ 0x01=IPv4, 0x04=IPv6, 0x03=Domain       │  │
│   │ Address         │ varies │ Address bytes (see below)               │  │
│   │ Port            │ 2      │ Destination port                        │  │
│   │ TTL             │ 1      │ Remaining hops (decremented each hop)   │  │
│   │ PathLength      │ 1      │ Number of agents in remaining path      │  │
│   │ RemainingPath   │ varies │ Array of AgentIDs (16 bytes each)       │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Address encoding:                                                         │
│   • IPv4 (0x01):   4 bytes, network order                                  │
│   • IPv6 (0x04):   16 bytes, network order                                 │
│   • Domain (0x03): 1-byte length + UTF-8 domain name                       │
│                                                                             │
│   Path processing:                                                          │
│   • If RemainingPath is empty, this agent is the exit                      │
│   • Otherwise, forward to first agent in RemainingPath                     │
│   • Remove self from path when forwarding                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN_ACK (0x02)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STREAM_OPEN_ACK                                   │
│                                                                             │
│   Stream opened successfully.                                              │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ RequestID       │ 8      │ Correlates to STREAM_OPEN               │  │
│   │ BoundAddrType   │ 1      │ Address type of actual connection       │  │
│   │ BoundAddr       │ varies │ Actual IP connected to                  │  │
│   │ BoundPort       │ 2      │ Actual port connected to                │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   BoundAddr is useful when domain was resolved at exit.                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_OPEN_ERR (0x03)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STREAM_OPEN_ERR                                   │
│                                                                             │
│   Stream open failed.                                                      │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ RequestID       │ 8      │ Correlates to STREAM_OPEN               │  │
│   │ ErrorCode       │ 2      │ Error code (see below)                  │  │
│   │ MessageLen      │ 1      │ Length of error message                 │  │
│   │ Message         │ varies │ Human-readable error (UTF-8)            │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Error codes:                                                              │
│   ┌───────┬──────────────────────┬────────────────────────────────────┐   │
│   │ Code  │ Name                 │ Meaning                            │   │
│   ├───────┼──────────────────────┼────────────────────────────────────┤   │
│   │ 1     │ NO_ROUTE             │ No route to destination            │   │
│   │ 2     │ CONNECTION_REFUSED   │ Target refused connection          │   │
│   │ 3     │ CONNECTION_TIMEOUT   │ Connection attempt timed out       │   │
│   │ 4     │ TTL_EXCEEDED         │ TTL reached zero                   │   │
│   │ 5     │ HOST_UNREACHABLE     │ Cannot reach target host           │   │
│   │ 6     │ NETWORK_UNREACHABLE  │ Cannot reach target network        │   │
│   │ 7     │ DNS_ERROR            │ Domain name resolution failed      │   │
│   │ 8     │ EXIT_DISABLED        │ Exit functionality not enabled     │   │
│   │ 9     │ RESOURCE_LIMIT       │ Too many streams                   │   │
│   └───────┴──────────────────────┴────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_DATA (0x04)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             STREAM_DATA                                     │
│                                                                             │
│   Payload data for a stream.                                               │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ Data            │ varies │ Raw payload bytes (max 16 KB)           │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   StreamID in frame header identifies the stream.                          │
│   Maximum payload: 16,384 bytes (enforced for fairness).                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_CLOSE (0x05)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            STREAM_CLOSE                                     │
│                                                                             │
│   Graceful stream close (supports half-close).                             │
│                                                                             │
│   Payload: Empty                                                           │
│   Behavior determined by Flags byte in frame header.                       │
│                                                                             │
│   Flag combinations:                                                        │
│   ┌───────┬─────────────────────────────────────────────────────────────┐  │
│   │ Flags │ Meaning                                                     │  │
│   ├───────┼─────────────────────────────────────────────────────────────┤  │
│   │ 0x01  │ FIN_WRITE: Sender done sending (half-close write)          │  │
│   │ 0x02  │ FIN_READ: Sender done receiving (half-close read)          │  │
│   │ 0x03  │ Both: Full close                                           │  │
│   └───────┴─────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   Half-close enables proper TCP FIN semantics for applications like SSH.  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### STREAM_RESET (0x06)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            STREAM_RESET                                     │
│                                                                             │
│   Abruptly terminate a stream with an error.                               │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ ErrorCode       │ 2      │ Error code (same as STREAM_OPEN_ERR)    │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Used when stream must be terminated abnormally (e.g., peer disconnect). │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ROUTE_ADVERTISE (0x10)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          ROUTE_ADVERTISE                                    │
│                                                                             │
│   Announce routes that an agent can reach.                                 │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ OriginAgent     │ 16     │ Agent that owns these routes            │  │
│   │ Sequence        │ 8      │ Monotonic sequence number               │  │
│   │ RouteCount      │ 1      │ Number of routes                        │  │
│   │ Routes          │ varies │ Array of Route entries                  │  │
│   │ PathLength      │ 1      │ Length of path from origin              │  │
│   │ Path            │ varies │ Array of AgentIDs (origin to here)      │  │
│   │ SeenByCount     │ 1      │ Number of agents that processed this    │  │
│   │ SeenBy          │ varies │ Array of AgentIDs for loop prevention   │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Route entry:                                                              │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ AddressFamily   │ 1      │ 0x01=IPv4, 0x02=IPv6                     │  │
│   │ PrefixLength    │ 1      │ CIDR prefix length (0-32 or 0-128)      │  │
│   │ Prefix          │ 4 or 16│ Network address bytes                   │  │
│   │ Metric          │ 2      │ Hop count from origin                   │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ROUTE_WITHDRAW (0x11)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ROUTE_WITHDRAW                                    │
│                                                                             │
│   Remove previously advertised routes.                                     │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ OriginAgent     │ 16     │ Agent withdrawing routes                │  │
│   │ Sequence        │ 8      │ Must be > last seen sequence            │  │
│   │ RouteCount      │ 1      │ Number of routes to withdraw            │  │
│   │ Routes          │ varies │ Array of Route entries (prefix only)    │  │
│   │ SeenByCount     │ 1      │ Number of agents that processed this    │  │
│   │ SeenBy          │ varies │ Array of AgentIDs for loop prevention   │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### KEEPALIVE (0x22) and KEEPALIVE_ACK (0x23)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        KEEPALIVE / KEEPALIVE_ACK                            │
│                                                                             │
│   Connection liveness probe and response.                                  │
│                                                                             │
│   ┌─────────────────┬────────┬──────────────────────────────────────────┐  │
│   │ Field           │ Size   │ Description                              │  │
│   ├─────────────────┼────────┼──────────────────────────────────────────┤  │
│   │ Timestamp       │ 8      │ Sender's Unix timestamp (for RTT calc)  │  │
│   └─────────────────┴────────┴──────────────────────────────────────────┘  │
│                                                                             │
│   Sent on StreamID 0 (control stream).                                     │
│   KEEPALIVE_ACK echoes the same timestamp.                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
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
│       ┌────────────┐     ┌────────────┐           │                        │
│       │ HALF_CLOSED│     │ HALF_CLOSED│           │                        │
│       │  (remote)  │     │  (local)   │           │                        │
│       └─────┬──────┘     └─────┬──────┘           │                        │
│             │                  │                  │                        │
│        Send CLOSE         Recv CLOSE              │                        │
│        (FIN_WRITE)        (FIN_WRITE)             │                        │
│             │                  │                  │                        │
│             └──────────────────┴──────────────────┘                        │
│                                │                                            │
│                                ▼                                            │
│                          ┌──────────┐                                       │
│                          │  CLOSED  │                                       │
│                          └──────────┘                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Half-Close Semantics

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          HALF-CLOSE SEMANTICS                               │
│                                                                             │
│  Half-close allows one side to signal "done sending" while still receiving.│
│  Critical for protocols like SSH that use TCP FIN for session termination. │
│                                                                             │
│  Example: SSH session exit                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  SSH Client           Mesh Network           SSH Server             │   │
│  │       │                    │                      │                 │   │
│  │       │                    │         "exit"       │                 │   │
│  │       │                    │◄─────────────────────│                 │   │
│  │       │   STREAM_DATA      │                      │                 │   │
│  │       │◄───────────────────│                      │                 │   │
│  │       │                    │    Exit status + FIN │                 │   │
│  │       │                    │◄─────────────────────│                 │   │
│  │       │   STREAM_DATA      │                      │                 │   │
│  │       │◄───────────────────│                      │                 │   │
│  │       │   STREAM_CLOSE     │                      │                 │   │
│  │       │   (FIN_WRITE)      │                      │                 │   │
│  │       │◄───────────────────│                      │                 │   │
│  │       │                    │                      │                 │   │
│  │       │   Client processes final output          │                 │   │
│  │       │   STREAM_CLOSE     │                      │                 │   │
│  │       │   (FIN_WRITE)      │                      │                 │   │
│  │       │───────────────────►│──────────────────────►                 │   │
│  │       │                    │                      │                 │   │
│  │       │        Stream fully closed               │                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Without half-close: Server "close" would immediately kill stream,         │
│  potentially losing final output and exit status.                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.3 Multi-Hop Stream Setup

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        MULTI-HOP STREAM SETUP                               │
│                                                                             │
│  SOCKS5 Client      Agent1         Agent2         Agent3        Target     │
│       │                │              │              │              │       │
│       │ CONNECT        │              │              │              │       │
│       │ 10.5.3.100:22  │              │              │              │       │
│       │───────────────►│              │              │              │       │
│       │                │              │              │              │       │
│       │                │ Route lookup: 10.5.3.100                   │       │
│       │                │ Result: Path=[Agent2,Agent3]               │       │
│       │                │              │              │              │       │
│       │                │ STREAM_OPEN  │              │              │       │
│       │                │ dst=10.5.3.100:22          │              │       │
│       │                │ path=[Agent3]│              │              │       │
│       │                │ ttl=15       │              │              │       │
│       │                │─────────────►│              │              │       │
│       │                │              │              │              │       │
│       │                │              │ STREAM_OPEN  │              │       │
│       │                │              │ dst=10.5.3.100:22          │       │
│       │                │              │ path=[]      │              │       │
│       │                │              │ ttl=14       │              │       │
│       │                │              │─────────────►│              │       │
│       │                │              │              │              │       │
│       │                │              │              │ Path empty,  │       │
│       │                │              │              │ I am exit    │       │
│       │                │              │              │              │       │
│       │                │              │              │ TCP connect  │       │
│       │                │              │              │─────────────►│       │
│       │                │              │              │◄─────────────│       │
│       │                │              │              │              │       │
│       │                │              │◄─────────────│              │       │
│       │                │              │ STREAM_OPEN_ACK             │       │
│       │                │◄─────────────│              │              │       │
│       │                │ STREAM_OPEN_ACK             │              │       │
│       │◄───────────────│              │              │              │       │
│       │ SOCKS5 SUCCESS │              │              │              │       │
│       │                │              │              │              │       │
│       │════════════════╪══════════════╪══════════════╪══════════════│       │
│       │                │  Bidirectional Data Flow    │              │       │
│       │════════════════╪══════════════╪══════════════╪══════════════│       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.4 Forward Table

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            FORWARD TABLE                                    │
│                                                                             │
│  Each agent maintains a forward table mapping streams to destinations.     │
│                                                                             │
│  Entry structure:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  ForwardEntry {                                                     │   │
│  │      IncomingPeer    AgentID       // Source peer                   │   │
│  │      IncomingStream  uint64        // Stream ID from source         │   │
│  │      OutgoingPeer    AgentID       // Destination peer (or empty)   │   │
│  │      OutgoingStream  uint64        // Stream ID to destination      │   │
│  │      LocalConn       net.Conn      // If exit: real TCP connection  │   │
│  │      State           StreamState   // OPEN, HALF_CLOSED, etc.       │   │
│  │      CreatedAt       time.Time                                      │   │
│  │      LastActivity    time.Time                                      │   │
│  │  }                                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Example on Agent2 (transit):                                               │
│  ┌───────────────────┬───────────────────┬─────────────────────────────┐   │
│  │ Incoming          │ Outgoing          │ Notes                       │   │
│  ├───────────────────┼───────────────────┼─────────────────────────────┤   │
│  │ Agent1:Stream5    │ Agent3:Stream12   │ SSH session, OPEN           │   │
│  │ Agent3:Stream12   │ Agent1:Stream5    │ Reverse mapping             │   │
│  │ Agent1:Stream7    │ Agent3:Stream14   │ Video stream, OPEN          │   │
│  │ Agent3:Stream14   │ Agent1:Stream7    │ Reverse mapping             │   │
│  └───────────────────┴───────────────────┴─────────────────────────────┘   │
│                                                                             │
│  Example on Agent3 (exit):                                                  │
│  ┌───────────────────┬───────────────────┬─────────────────────────────┐   │
│  │ Incoming          │ Local Connection  │ Notes                       │   │
│  ├───────────────────┼───────────────────┼─────────────────────────────┤   │
│  │ Agent2:Stream12   │ TCP 10.5.3.100:22 │ SSH session to server       │   │
│  │ Agent2:Stream14   │ TCP 10.5.3.50:443 │ Video stream to server      │   │
│  └───────────────────┴───────────────────┴─────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.5 Resource Limits

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           RESOURCE LIMITS                                   │
│                                                                             │
│  Default limits (configurable):                                             │
│                                                                             │
│  ┌─────────────────────────────┬────────────┬───────────────────────────┐  │
│  │ Resource                    │ Default    │ Notes                     │  │
│  ├─────────────────────────────┼────────────┼───────────────────────────┤  │
│  │ Max streams per peer        │ 1,000      │ Incoming + outgoing       │  │
│  │ Max total streams           │ 10,000     │ All peers combined        │  │
│  │ Max pending opens           │ 100        │ STREAM_OPEN awaiting ACK  │  │
│  │ Stream open timeout         │ 30s        │ Time to receive ACK       │  │
│  │ Idle stream timeout         │ 5m         │ No data exchanged         │  │
│  │ Read buffer per stream      │ 256 KB     │                           │  │
│  │ Write buffer per stream     │ 256 KB     │                           │  │
│  └─────────────────────────────┴────────────┴───────────────────────────┘  │
│                                                                             │
│  When limits are reached:                                                   │
│  • New STREAM_OPEN receives STREAM_OPEN_ERR with RESOURCE_LIMIT            │
│  • Existing streams are not affected                                       │
│  • Log warning for monitoring                                              │
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
│      Prefix      net.IPNet     // CIDR prefix (e.g., 10.0.0.0/8)          │
│      NextHop     AgentID       // Immediate peer to forward to             │
│      ExitAgent   AgentID       // Final agent that reaches this prefix     │
│      Path        []AgentID     // Full path from here to exit              │
│      Metric      uint16        // Hop count                                │
│      Sequence    uint64        // From origin's advertisement              │
│      ExpiresAt   time.Time     // When this route becomes invalid          │
│  }                                                                          │
│                                                                             │
│  RouteTable {                                                               │
│      entries     []*RouteEntry // Sorted by prefix length (longest first) │
│      byOrigin    map[AgentID][]*RouteEntry                                 │
│      lock        sync.RWMutex                                              │
│  }                                                                          │
│                                                                             │
│  Route entries are sorted by prefix length (descending) for efficient      │
│  longest-prefix match during lookup.                                       │
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
│  ┌──────────────────┬──────────┬──────────┬────────┬────────────────────┐  │
│  │ Prefix           │ NextHop  │ Exit     │ Metric │ Path               │  │
│  ├──────────────────┼──────────┼──────────┼────────┼────────────────────┤  │
│  │ 10.5.3.0/24      │ Agent2   │ Agent3   │ 2      │ [Agent2, Agent3]   │  │
│  │ 10.5.0.0/16      │ Agent2   │ Agent4   │ 3      │ [Agent2, Agent3,   │  │
│  │                  │          │          │        │  Agent4]           │  │
│  │ 10.0.0.0/8       │ Agent2   │ Agent3   │ 2      │ [Agent2, Agent3]   │  │
│  │ 192.168.0.0/16   │ Agent2   │ Agent3   │ 2      │ [Agent2, Agent3]   │  │
│  │ 0.0.0.0/0        │ Agent5   │ Agent5   │ 1      │ [Agent5]           │  │
│  └──────────────────┴──────────┴──────────┴────────┴────────────────────┘  │
│                                                                             │
│  Lookup examples:                                                           │
│                                                                             │
│  Lookup(10.5.3.100):                                                        │
│    Matches: 10.5.3.0/24 (24 bits) ← Selected (longest prefix)             │
│             10.5.0.0/16 (16 bits)                                          │
│             10.0.0.0/8 (8 bits)                                            │
│             0.0.0.0/0 (0 bits)                                             │
│    Result: Path=[Agent2, Agent3]                                           │
│                                                                             │
│  Lookup(10.5.4.50):                                                         │
│    Matches: 10.5.0.0/16 (16 bits) ← Selected (longest prefix)             │
│             10.0.0.0/8 (8 bits)                                            │
│             0.0.0.0/0 (0 bits)                                             │
│    Result: Path=[Agent2, Agent3, Agent4]                                   │
│                                                                             │
│  Lookup(8.8.8.8):                                                           │
│    Matches: 0.0.0.0/0 (0 bits) ← Selected (only match)                    │
│    Result: Path=[Agent5]                                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.3 Route Expiration

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          ROUTE EXPIRATION                                   │
│                                                                             │
│  Routes have a TTL and expire if not refreshed.                            │
│                                                                             │
│  Timing parameters:                                                         │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐  │
│  │ Parameter                   │ Default  │ Description                 │  │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤  │
│  │ Route TTL                   │ 5m       │ How long routes are valid   │  │
│  │ Advertise interval          │ 2m       │ How often to re-advertise   │  │
│  │ Cleanup interval            │ 30s      │ How often to check expiry   │  │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘  │
│                                                                             │
│  Route lifecycle:                                                           │
│  1. Exit agent advertises routes every 2 minutes                           │
│  2. Advertisement propagates through mesh                                  │
│  3. Each agent sets ExpiresAt = now + 5 minutes                           │
│  4. Cleanup goroutine removes expired routes every 30 seconds             │
│  5. If route expires before refresh, traffic fails (no route)             │
│                                                                             │
│  On peer disconnect:                                                        │
│  • Immediately remove all routes where NextHop = disconnected peer         │
│  • Don't wait for expiration                                               │
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
│  Agent3 is exit for 10.0.0.0/8                                             │
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
│          │                        │ path=[Agent3]          │                │
│          │                        │ metric=1               │                │
│          │                        │                        │                │
│          │                        │ ROUTE_ADVERTISE        │                │
│          │                        │ origin=Agent3          │                │
│          │                        │ seq=1                  │                │
│          │                        │ routes=[10.0.0.0/8]    │                │
│          │                        │ metric=1               │                │
│          │                        │ path=[Agent2,Agent3]   │                │
│          │                        │ seenBy=[Agent3,Agent2] │                │
│          │                        │───────────────────────►│                │
│          │                        │                        │                │
│          │                        │                        │ Store route:   │
│          │                        │                        │ 10.0.0.0/8     │
│          │                        │                        │ nextHop=Agent2 │
│          │                        │                        │ exit=Agent3    │
│          │                        │                        │ path=[Agent2,  │
│          │                        │                        │       Agent3]  │
│          │                        │                        │ metric=2       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Flood Algorithm

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FLOOD ALGORITHM                                   │
│                                                                             │
│  OnReceive(ROUTE_ADVERTISE, fromPeer):                                     │
│                                                                             │
│      // 1. Loop prevention                                                 │
│      if msg.OriginAgent == self.ID:                                        │
│          return  // Our own advertisement came back                        │
│                                                                             │
│      if self.ID in msg.SeenBy:                                             │
│          return  // Already processed this                                 │
│                                                                             │
│      // 2. Freshness check                                                 │
│      lastSeq := sequenceTracker[msg.OriginAgent]                           │
│      if msg.Sequence <= lastSeq:                                           │
│          return  // Stale update                                           │
│      sequenceTracker[msg.OriginAgent] = msg.Sequence                       │
│                                                                             │
│      // 3. Store routes                                                    │
│      for route in msg.Routes:                                              │
│          entry := RouteEntry{                                              │
│              Prefix:    route.Prefix,                                      │
│              NextHop:   fromPeer,                                          │
│              ExitAgent: msg.OriginAgent,                                   │
│              Path:      prepend(fromPeer, msg.Path),                       │
│              Metric:    route.Metric + 1,                                  │
│              Sequence:  msg.Sequence,                                      │
│              ExpiresAt: now() + routeTTL,                                  │
│          }                                                                 │
│          routeTable.Update(entry)                                          │
│                                                                             │
│      // 4. Forward to other peers (split horizon)                          │
│      msg.SeenBy = append(msg.SeenBy, self.ID)                              │
│      msg.Path = prepend(self.ID, msg.Path)                                 │
│      for route in msg.Routes:                                              │
│          route.Metric++                                                    │
│                                                                             │
│      for peer in connectedPeers:                                           │
│          if peer != fromPeer && peer not in msg.SeenBy:                    │
│              send(peer, msg)                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 9.3 Local Route Advertisement

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      LOCAL ROUTE ADVERTISEMENT                              │
│                                                                             │
│  Exit agents periodically advertise their local routes.                    │
│                                                                             │
│  Triggered by:                                                              │
│  • Startup (after connecting to peers)                                     │
│  • Timer (every advertise interval)                                        │
│  • Configuration change (routes added/removed)                             │
│                                                                             │
│  func advertiseLocalRoutes():                                               │
│      if !config.Exit.Enabled || len(config.Exit.Routes) == 0:              │
│          return                                                            │
│                                                                             │
│      localSequence++                                                       │
│                                                                             │
│      msg := ROUTE_ADVERTISE{                                               │
│          OriginAgent: self.ID,                                             │
│          Sequence:    localSequence,                                       │
│          Routes:      config.Exit.Routes,  // All with Metric=0           │
│          Path:        [self.ID],                                           │
│          SeenBy:      [self.ID],                                           │
│      }                                                                     │
│                                                                             │
│      for peer in connectedPeers:                                           │
│          send(peer, msg)                                                   │
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
│       │ [Transport connects - QUIC/HTTP2/WS]        │                      │
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
│       │ Validate:                                    │                      │
│       │ • Version compatible                         │                      │
│       │ • AgentID expected?                          │                      │
│       │                                              │                      │
│       │              Connection ready                │                      │
│       │                                              │                      │
│                                                                             │
│  Handshake timeout: 10 seconds                                             │
│  If no PEER_HELLO received within timeout, close connection.               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Reconnection Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        RECONNECTION STRATEGY                                │
│                                                                             │
│  Exponential backoff with jitter:                                          │
│                                                                             │
│  Parameters:                                                                │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐  │
│  │ Parameter                   │ Default  │ Description                 │  │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤  │
│  │ Initial delay               │ 1s       │ First reconnect attempt     │  │
│  │ Max delay                   │ 60s      │ Cap on backoff              │  │
│  │ Multiplier                  │ 2.0      │ Exponential factor          │  │
│  │ Jitter                      │ 0.2      │ ±20% randomization          │  │
│  │ Max retries                 │ 0        │ 0 = infinite                │  │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘  │
│                                                                             │
│  Retry sequence:                                                            │
│    Attempt 1:  1s    ± 0.2s  →  0.8s  - 1.2s                              │
│    Attempt 2:  2s    ± 0.4s  →  1.6s  - 2.4s                              │
│    Attempt 3:  4s    ± 0.8s  →  3.2s  - 4.8s                              │
│    Attempt 4:  8s    ± 1.6s  →  6.4s  - 9.6s                              │
│    Attempt 5:  16s   ± 3.2s  →  12.8s - 19.2s                             │
│    Attempt 6:  32s   ± 6.4s  →  25.6s - 38.4s                             │
│    Attempt 7+: 60s   ± 12s   →  48s   - 72s  (capped)                     │
│                                                                             │
│  On successful reconnect:                                                   │
│  • Reset backoff to initial delay                                          │
│  • Perform handshake                                                       │
│  • Request fresh route advertisements                                      │
│  • All previous streams are dead (do not attempt to resume)                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.4 Keepalive Mechanism

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         KEEPALIVE MECHANISM                                 │
│                                                                             │
│  Purpose:                                                                   │
│  • Detect dead connections                                                 │
│  • Maintain NAT/firewall mappings                                          │
│  • Measure round-trip time                                                 │
│                                                                             │
│  Parameters:                                                                │
│  ┌─────────────────────────────┬──────────┬─────────────────────────────┐  │
│  │ Parameter                   │ Default  │ Description                 │  │
│  ├─────────────────────────────┼──────────┼─────────────────────────────┤  │
│  │ Idle threshold              │ 30s      │ Send keepalive after idle   │  │
│  │ Timeout                     │ 90s      │ Declare dead if no response │  │
│  └─────────────────────────────┴──────────┴─────────────────────────────┘  │
│                                                                             │
│  Algorithm:                                                                 │
│                                                                             │
│  lastActivity := now()                                                     │
│                                                                             │
│  // On any frame sent or received:                                         │
│  lastActivity = now()                                                      │
│                                                                             │
│  // Periodic check (every 10s):                                            │
│  if now() - lastActivity > idleThreshold:                                  │
│      send KEEPALIVE                                                        │
│                                                                             │
│  if now() - lastActivity > timeout:                                        │
│      mark connection as dead                                               │
│      trigger reconnection                                                  │
│                                                                             │
│  // On KEEPALIVE received:                                                 │
│  send KEEPALIVE_ACK (echo timestamp)                                       │
│  lastActivity = now()                                                      │
│                                                                             │
│  // On KEEPALIVE_ACK received:                                             │
│  rtt = now() - ack.Timestamp                                               │
│  lastActivity = now()                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.5 Connection Teardown

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CONNECTION TEARDOWN                                 │
│                                                                             │
│  When a peer connection is lost (error, timeout, or graceful close):       │
│                                                                             │
│  1. Mark connection as disconnected                                        │
│                                                                             │
│  2. Clean up streams:                                                      │
│     for entry in forwardTable:                                             │
│         if entry.IncomingPeer == peer || entry.OutgoingPeer == peer:       │
│             if entry.LocalConn != nil:                                     │
│                 entry.LocalConn.Close()  // Close exit TCP connection     │
│             if entry.OtherPeer != nil:                                     │
│                 send STREAM_RESET to other peer                            │
│             remove entry from forwardTable                                 │
│                                                                             │
│  3. Clean up routes:                                                       │
│     for route in routeTable:                                               │
│         if route.NextHop == peer:                                          │
│             remove route                                                   │
│                                                                             │
│  4. Trigger reconnection (if configured)                                   │
│                                                                             │
│  Important: Streams do NOT survive reconnection.                           │
│  Applications must handle reconnection (e.g., SSH session dies).           │
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
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • CONNECT command (TCP proxy)                                       │   │
│  │ • IPv4 addresses                                                    │   │
│  │ • IPv6 addresses                                                    │   │
│  │ • Domain names (resolved at exit agent)                             │   │
│  │ • No authentication (method 0x00)                                   │   │
│  │ • Username/password authentication (method 0x02)                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Not supported:                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • BIND command (incoming connections)                               │   │
│  │ • UDP ASSOCIATE command (UDP proxy)                                 │   │
│  │ • GSSAPI authentication (method 0x01)                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.2 Connection Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        SOCKS5 CONNECTION FLOW                               │
│                                                                             │
│  Client                                SOCKS5 Handler                       │
│     │                                        │                              │
│     │ Version: 05                            │                              │
│     │ NMethods: 01                           │                              │
│     │ Methods: [00]  (no auth)               │                              │
│     │───────────────────────────────────────►│                              │
│     │                                        │                              │
│     │                         Version: 05    │                              │
│     │                         Method: 00     │                              │
│     │◄───────────────────────────────────────│                              │
│     │                                        │                              │
│     │ Version: 05                            │                              │
│     │ Command: 01 (CONNECT)                  │                              │
│     │ Reserved: 00                           │                              │
│     │ AddrType: 03 (domain)                  │                              │
│     │ Address: "server.internal"             │                              │
│     │ Port: 22                               │                              │
│     │───────────────────────────────────────►│                              │
│     │                                        │                              │
│     │                                        │ 1. Route lookup              │
│     │                                        │ 2. Open virtual stream       │
│     │                                        │ 3. Wait for ACK              │
│     │                                        │                              │
│     │                         Version: 05    │                              │
│     │                         Reply: 00 (OK) │                              │
│     │                         Reserved: 00   │                              │
│     │                         AddrType: 01   │                              │
│     │                         BoundAddr: ... │                              │
│     │                         BoundPort: ... │                              │
│     │◄───────────────────────────────────────│                              │
│     │                                        │                              │
│     │◄═══════════════════════════════════════│                              │
│     │         Bidirectional data relay       │                              │
│     │═══════════════════════════════════════►│                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.3 Error Mapping

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SOCKS5 ERROR MAPPING                                │
│                                                                             │
│  ┌─────────────────────────────┬────────────────────────────────────────┐  │
│  │ Internal Error              │ SOCKS5 Reply                           │  │
│  ├─────────────────────────────┼────────────────────────────────────────┤  │
│  │ NO_ROUTE                    │ 0x04 Host unreachable                  │  │
│  │ CONNECTION_REFUSED          │ 0x05 Connection refused                │  │
│  │ CONNECTION_TIMEOUT          │ 0x06 TTL expired                       │  │
│  │ TTL_EXCEEDED                │ 0x06 TTL expired                       │  │
│  │ HOST_UNREACHABLE            │ 0x04 Host unreachable                  │  │
│  │ NETWORK_UNREACHABLE         │ 0x03 Network unreachable               │  │
│  │ DNS_ERROR                   │ 0x04 Host unreachable                  │  │
│  │ EXIT_DISABLED               │ 0x01 General failure                   │  │
│  │ RESOURCE_LIMIT              │ 0x01 General failure                   │  │
│  │ Any other error             │ 0x01 General failure                   │  │
│  └─────────────────────────────┴────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.4 Handler Implementation

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      SOCKS5 HANDLER PSEUDOCODE                              │
│                                                                             │
│  func handleSOCKS5(clientConn net.Conn):                                   │
│      defer clientConn.Close()                                              │
│                                                                             │
│      // 1. Greeting                                                        │
│      methods := readGreeting(clientConn)                                   │
│      if !supportsMethod(methods):                                          │
│          writeNoAcceptableMethods(clientConn)                              │
│          return                                                            │
│      writeMethodSelection(clientConn, selectedMethod)                      │
│                                                                             │
│      // 2. Authentication (if required)                                    │
│      if selectedMethod == USERNAME_PASSWORD:                               │
│          if !authenticate(clientConn):                                     │
│              return                                                        │
│                                                                             │
│      // 3. Request                                                         │
│      req := readRequest(clientConn)                                        │
│      if req.Command != CONNECT:                                            │
│          writeReply(clientConn, COMMAND_NOT_SUPPORTED)                     │
│          return                                                            │
│                                                                             │
│      // 4. Route lookup                                                    │
│      route := routeTable.Lookup(req.Address)                               │
│      if route == nil:                                                      │
│          writeReply(clientConn, HOST_UNREACHABLE)                          │
│          return                                                            │
│                                                                             │
│      // 5. Open virtual stream                                             │
│      stream, err := openStream(route.Path, req.Address, req.Port)          │
│      if err != nil:                                                        │
│          writeReply(clientConn, mapError(err))                             │
│          return                                                            │
│                                                                             │
│      // 6. Success                                                         │
│      writeReply(clientConn, SUCCESS, stream.BoundAddr, stream.BoundPort)   │
│                                                                             │
│      // 7. Bidirectional relay                                             │
│      relay(clientConn, stream)                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 12. Data Plane

### 12.1 Frame Processing

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         FRAME PROCESSING                                    │
│                                                                             │
│  Incoming frame from peer:                                                  │
│                                                                             │
│  func processFrame(peer *PeerConn, frame *Frame):                          │
│      switch frame.Type:                                                    │
│                                                                             │
│      case STREAM_OPEN:                                                     │
│          handleStreamOpen(peer, frame)                                     │
│                                                                             │
│      case STREAM_OPEN_ACK, STREAM_OPEN_ERR:                                │
│          handleStreamOpenResponse(peer, frame)                             │
│                                                                             │
│      case STREAM_DATA:                                                     │
│          entry := forwardTable.Lookup(peer.ID, frame.StreamID)             │
│          if entry == nil:                                                  │
│              send STREAM_RESET (unknown stream)                            │
│              return                                                        │
│          if entry.LocalConn != nil:                                        │
│              // Exit: write to real TCP connection                         │
│              entry.LocalConn.Write(frame.Payload)                          │
│          else:                                                             │
│              // Transit: forward to next peer                              │
│              forward(entry.OutgoingPeer, entry.OutgoingStreamID, frame)    │
│                                                                             │
│      case STREAM_CLOSE:                                                    │
│          handleStreamClose(peer, frame)                                    │
│                                                                             │
│      case STREAM_RESET:                                                    │
│          handleStreamReset(peer, frame)                                    │
│                                                                             │
│      case ROUTE_ADVERTISE:                                                 │
│          floodProtocol.HandleAdvertise(peer, frame)                        │
│                                                                             │
│      case ROUTE_WITHDRAW:                                                  │
│          floodProtocol.HandleWithdraw(peer, frame)                         │
│                                                                             │
│      case KEEPALIVE:                                                       │
│          send KEEPALIVE_ACK with same timestamp                            │
│                                                                             │
│      case KEEPALIVE_ACK:                                                   │
│          updateRTT(peer, frame.Timestamp)                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 12.2 Write Fairness

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          WRITE FAIRNESS                                     │
│                                                                             │
│  Problem: One stream (video) could monopolize the transport, starving      │
│  latency-sensitive streams (SSH).                                          │
│                                                                             │
│  Solution for HTTP/2 and WebSocket (single transport stream):              │
│                                                                             │
│  Writer maintains:                                                          │
│  • Per-stream outgoing queue                                               │
│  • Round-robin scheduler                                                   │
│                                                                             │
│  Algorithm:                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  // Maximum data per frame                                          │   │
│  │  const maxFramePayload = 16384  // 16 KB                           │   │
│  │                                                                     │   │
│  │  // Writer loop                                                     │   │
│  │  for {                                                              │   │
│  │      // Get streams with pending data (round-robin order)          │   │
│  │      streams := scheduler.GetReadyStreams()                         │   │
│  │      if len(streams) == 0 {                                         │   │
│  │          wait for data                                              │   │
│  │          continue                                                   │   │
│  │      }                                                              │   │
│  │                                                                     │   │
│  │      for _, stream := range streams {                               │   │
│  │          // Send at most one frame per stream per round            │   │
│  │          data := stream.queue.Read(maxFramePayload)                 │   │
│  │          frame := createDataFrame(stream.id, data)                  │   │
│  │          transport.Write(frame)                                     │   │
│  │      }                                                              │   │
│  │  }                                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  QUIC transport: Not needed (native per-stream fairness).                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 12.3 Bidirectional Relay

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        BIDIRECTIONAL RELAY                                  │
│                                                                             │
│  Used at:                                                                   │
│  • SOCKS5 handler (client ↔ virtual stream)                                │
│  • Exit handler (virtual stream ↔ real TCP)                                │
│                                                                             │
│  func relay(a, b io.ReadWriteCloser):                                      │
│      done := make(chan struct{}, 2)                                        │
│                                                                             │
│      // a → b                                                              │
│      go func() {                                                           │
│          io.Copy(b, a)                                                     │
│          b.CloseWrite()  // Half-close                                     │
│          done <- struct{}{}                                                │
│      }()                                                                   │
│                                                                             │
│      // b → a                                                              │
│      go func() {                                                           │
│          io.Copy(a, b)                                                     │
│          a.CloseWrite()  // Half-close                                     │
│          done <- struct{}{}                                                │
│      }()                                                                   │
│                                                                             │
│      // Wait for both directions to complete                               │
│      <-done                                                                │
│      <-done                                                                │
│                                                                             │
│      // Full close                                                         │
│      a.Close()                                                             │
│      b.Close()                                                             │
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

  # Directory for persistent state
  data_dir: "./data"

  # Logging
  log_level: "info" # debug, info, warn, error
  log_format: "text" # text, json

# ------------------------------------------------------------------------------
# Transport Listeners
# ------------------------------------------------------------------------------
listeners:
  # QUIC listener (best performance)
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt" # Optional: require client certs

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
      # Or use pinning:
      # fingerprint: "sha256:ab12cd34..."

  # HTTP/2 peer
  - id: "def456..."
    transport: h2
    address: "https://gateway.example.com:8443/mesh"
    tls:
      ca: "./certs/ca.crt"

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
  route_ttl: 5m
  max_hops: 16

# ------------------------------------------------------------------------------
# Connection Tuning
# ------------------------------------------------------------------------------
connections:
  # Keepalive
  idle_threshold: 30s
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
```

### 13.2 Environment Variable Substitution

Configuration values support environment variable substitution:

```yaml
peers:
  - id: "${PEER_ID}"
    address: "${PEER_ADDRESS}"
    proxy_auth:
      password: "${PROXY_PASSWORD}"
```

### 13.3 Command-Line Interface

```bash
# Initialize new agent
mesh-agent init --data-dir ./data

# Run agent
mesh-agent run --config ./config.yaml

# Run with overrides
mesh-agent run --config ./config.yaml \
    --socks5.address=127.0.0.1:9050 \
    --log-level=debug

# Show status
mesh-agent status

# List peers
mesh-agent peers

# List routes
mesh-agent routes

# Test route
mesh-agent route-lookup 10.5.3.100
```

---

## 14. Security

### 14.1 Transport Security

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         TRANSPORT SECURITY                                  │
│                                                                             │
│  All transports use TLS:                                                    │
│                                                                             │
│  ┌─────────────────┬─────────────────────────────────────────────────────┐ │
│  │ Transport       │ TLS Implementation                                  │ │
│  ├─────────────────┼─────────────────────────────────────────────────────┤ │
│  │ QUIC            │ TLS 1.3 (mandatory, built into QUIC)               │ │
│  │ HTTP/2          │ TLS 1.2+ (required in practice)                    │ │
│  │ WebSocket       │ TLS 1.2+ (WSS)                                     │ │
│  └─────────────────┴─────────────────────────────────────────────────────┘ │
│                                                                             │
│  Certificate validation options:                                            │
│  • CA-based: Validate against trusted CA certificate                       │
│  • Pinning: Validate against specific certificate fingerprint              │
│  • Mutual TLS: Both sides present and validate certificates                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 14.2 Trust Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            TRUST MODEL                                      │
│                                                                             │
│  Direct trust:                                                              │
│  • Agent trusts directly connected peers                                   │
│  • Verified via TLS and AgentID                                            │
│                                                                             │
│  Transitive trust:                                                          │
│  • Agent trusts route advertisements from direct peers                     │
│  • These include routes from agents further in the mesh                    │
│  • No direct verification of distant agents                                │
│                                                                             │
│  Implications:                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ • A malicious peer can advertise false routes                       │   │
│  │ • Traffic could be misdirected                                      │   │
│  │ • Mitigation: Only connect to trusted peers                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  What transit agents can see:                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Visible:                                                            │   │
│  │ • Destination IP/port (in STREAM_OPEN)                              │   │
│  │ • Traffic volume and timing                                         │   │
│  │ • Path information                                                  │   │
│  │                                                                     │   │
│  │ Not visible:                                                        │   │
│  │ • Payload content (just forwarded)                                  │   │
│  │ • Application-layer data                                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  End-to-end encryption:                                                     │
│  • The mesh provides transport security (TLS between each hop)             │
│  • For true end-to-end encryption, use application-layer TLS              │
│    (e.g., HTTPS, SSH)                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 14.3 SOCKS5 Security

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SOCKS5 SECURITY                                     │
│                                                                             │
│  Recommendations:                                                           │
│                                                                             │
│  1. Bind to localhost only                                                 │
│     address: "127.0.0.1:1080"                                              │
│     Prevents remote access to SOCKS5 proxy                                 │
│                                                                             │
│  2. Enable authentication                                                  │
│     auth:                                                                  │
│       enabled: true                                                        │
│       users: [...]                                                         │
│     Prevents unauthorized local use                                        │
│                                                                             │
│  3. Use firewall rules                                                     │
│     Block external access to SOCKS5 port even if misconfigured             │
│                                                                             │
│  4. Connection limits                                                      │
│     max_connections: 1000                                                  │
│     Prevents resource exhaustion                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 15. Project Structure

### 15.1 Directory Layout

```
mesh-agent/
├── cmd/
│   └── mesh-agent/
│       ├── main.go                 # CLI entrypoint
│       ├── run.go                  # Run command
│       ├── init.go                 # Init command
│       └── status.go               # Status commands
│
├── internal/
│   ├── agent/
│   │   ├── agent.go                # Main agent orchestration
│   │   └── options.go              # Agent options
│   │
│   ├── config/
│   │   ├── config.go               # Configuration parsing
│   │   └── validate.go             # Configuration validation
│   │
│   ├── identity/
│   │   └── identity.go             # AgentID generation/storage
│   │
│   ├── transport/
│   │   ├── transport.go            # Transport interface
│   │   ├── quic.go                 # QUIC implementation
│   │   ├── h2.go                   # HTTP/2 implementation
│   │   ├── ws.go                   # WebSocket implementation
│   │   └── tls.go                  # TLS helpers
│   │
│   ├── peer/
│   │   ├── manager.go              # Peer lifecycle management
│   │   ├── connection.go           # Single peer connection
│   │   ├── handshake.go            # PEER_HELLO handling
│   │   └── reconnect.go            # Reconnection logic
│   │
│   ├── protocol/
│   │   ├── frame.go                # Frame encode/decode
│   │   ├── types.go                # Message type definitions
│   │   ├── reader.go               # Frame reader
│   │   └── writer.go               # Frame writer (with fairness)
│   │
│   ├── stream/
│   │   ├── manager.go              # Stream lifecycle
│   │   ├── forward.go              # Forward table
│   │   └── virtual.go              # Virtual stream abstraction
│   │
│   ├── routing/
│   │   ├── table.go                # Route table
│   │   └── lookup.go               # Longest prefix match
│   │
│   ├── flood/
│   │   ├── flood.go                # Flood protocol
│   │   ├── advertise.go            # Route advertisement
│   │   └── withdraw.go             # Route withdrawal
│   │
│   ├── socks5/
│   │   ├── server.go               # SOCKS5 server
│   │   ├── handler.go              # Connection handler
│   │   └── auth.go                 # Authentication
│   │
│   └── exit/
│       ├── handler.go              # Exit handler
│       └── dns.go                  # DNS resolution
│
├── pkg/
│   └── cidr/
│       └── cidr.go                 # CIDR utilities
│
├── configs/
│   └── example.yaml                # Example configuration
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 15.2 Dependencies

| Package                      | Purpose                     |
| ---------------------------- | --------------------------- |
| `github.com/quic-go/quic-go` | QUIC transport              |
| `golang.org/x/net/http2`     | HTTP/2 transport            |
| `nhooyr.io/websocket`        | WebSocket transport         |
| `gopkg.in/yaml.v3`           | Configuration parsing       |
| `github.com/spf13/cobra`     | CLI framework               |
| `log/slog`                   | Structured logging (stdlib) |

---

## 16. Implementation Notes

### 16.1 Critical Settings

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CRITICAL SETTINGS                                    │
│                                                                             │
│  TCP_NODELAY (Nagle's algorithm):                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  MUST disable Nagle's algorithm on all TCP connections.             │   │
│  │  Without this, small packets (SSH keystrokes) are delayed.          │   │
│  │                                                                     │   │
│  │  Go: conn.(*net.TCPConn).SetNoDelay(true)                          │   │
│  │                                                                     │   │
│  │  Apply to:                                                          │   │
│  │  • HTTP/2 transport connections                                     │   │
│  │  • WebSocket transport connections                                  │   │
│  │  • Exit handler TCP connections                                     │   │
│  │  • SOCKS5 client connections                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Buffer sizes:                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Default: 256 KB per stream                                         │   │
│  │  Suitable for video streaming                                       │   │
│  │  Configurable for memory-constrained environments                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Frame size:                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Maximum: 16 KB payload                                             │   │
│  │  Ensures fairness between streams                                   │   │
│  │  Prevents one stream from blocking others                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 16.2 Goroutine Management

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       GOROUTINE MANAGEMENT                                  │
│                                                                             │
│  Per peer connection:                                                       │
│  • 1 reader goroutine (reads frames, dispatches)                           │
│  • 1 writer goroutine (serializes outgoing frames)                         │
│  • 1 keepalive goroutine (periodic checks)                                 │
│                                                                             │
│  Per stream (at endpoints):                                                 │
│  • 2 goroutines for bidirectional relay                                    │
│                                                                             │
│  Leak prevention:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • Use context.Context for cancellation                             │   │
│  │  • Set read/write deadlines                                         │   │
│  │  • Close underlying connections on shutdown                         │   │
│  │  • Log goroutine count periodically (debug mode)                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Periodic cleanup (every 5 minutes):                                        │
│  • Remove expired routes                                                   │
│  • Remove stale forward table entries                                      │
│  • Log resource usage                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 16.3 Error Handling

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ERROR HANDLING                                      │
│                                                                             │
│  Connection errors:                                                         │
│  • Log error with peer ID                                                  │
│  • Clean up streams (send STREAM_RESET)                                    │
│  • Remove routes via this peer                                             │
│  • Trigger reconnection                                                    │
│                                                                             │
│  Stream errors:                                                             │
│  • Log error with stream ID                                                │
│  • Send STREAM_RESET to both ends                                          │
│  • Remove from forward table                                               │
│  • Close local connections if any                                          │
│                                                                             │
│  Configuration errors:                                                      │
│  • Validate on startup                                                     │
│  • Fail fast with clear error message                                      │
│                                                                             │
│  Resource exhaustion:                                                       │
│  • Reject new streams with RESOURCE_LIMIT error                            │
│  • Log warning                                                             │
│  • Don't kill existing streams                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 17. Testing Strategy

### 17.1 Test Levels

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          TEST LEVELS                                        │
│                                                                             │
│  Unit Tests:                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • Frame encode/decode                                              │   │
│  │  • Route table operations (insert, lookup, expire)                  │   │
│  │  • Stream state machine transitions                                 │   │
│  │  • CIDR matching                                                    │   │
│  │  • Configuration parsing                                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Integration Tests:                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • 2-agent chain (ingress → exit)                                   │   │
│  │  • 3-agent chain (ingress → transit → exit)                         │   │
│  │  • Each transport type (QUIC, HTTP/2, WebSocket)                    │   │
│  │  • Reconnection behavior                                            │   │
│  │  • Route propagation                                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  End-to-End Tests:                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  • SSH session through 3-agent chain                                │   │
│  │  • HTTP request/response                                            │   │
│  │  • Large file transfer                                              │   │
│  │  • Concurrent streams (SSH + video simulation)                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 17.2 Test Environment

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TEST ENVIRONMENT                                     │
│                                                                             │
│  Docker Compose setup:                                                      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  services:                                                          │   │
│  │    agent1:                                                          │   │
│  │      # Ingress (SOCKS5)                                            │   │
│  │      image: mesh-agent                                              │   │
│  │      ports: ["1080:1080"]                                           │   │
│  │                                                                     │   │
│  │    agent2:                                                          │   │
│  │      # Transit                                                      │   │
│  │      image: mesh-agent                                              │   │
│  │                                                                     │   │
│  │    agent3:                                                          │   │
│  │      # Exit                                                         │   │
│  │      image: mesh-agent                                              │   │
│  │                                                                     │   │
│  │    target:                                                          │   │
│  │      # Test target (SSH server, HTTP server)                       │   │
│  │      image: test-target                                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Test scenarios:                                                            │
│  • Basic connectivity: curl through SOCKS5                                 │
│  • SSH session: Interactive shell, file transfer                           │
│  • Reconnection: Kill agent2, verify recovery                              │
│  • Latency: Measure RTT through chain                                      │
│  • Throughput: iperf3 through chain                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 17.3 Manual Testing Checklist

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      MANUAL TESTING CHECKLIST                               │
│                                                                             │
│  Basic functionality:                                                       │
│  □ Agent starts with valid configuration                                   │
│  □ Agent rejects invalid configuration with clear error                    │
│  □ Peers connect and complete handshake                                    │
│  □ Routes propagate through mesh                                           │
│  □ SOCKS5 proxy accepts connections                                        │
│  □ HTTP request succeeds through chain                                     │
│                                                                             │
│  SSH testing (latency-sensitive):                                           │
│  □ SSH connection establishes                                              │
│  □ Interactive shell is responsive (no typing lag)                         │
│  □ File transfer works (scp)                                               │
│  □ Session survives idle time                                              │
│  □ Exit command properly terminates session                                │
│                                                                             │
│  Video/bulk testing (bandwidth):                                            │
│  □ Large file download completes                                           │
│  □ Throughput is reasonable (measure with iperf3)                          │
│  □ SSH remains responsive during bulk transfer                             │
│                                                                             │
│  Failure testing:                                                           │
│  □ Agent recovers after network interruption                               │
│  □ Streams fail gracefully when path breaks                                │
│  □ Routes expire and are re-advertised                                     │
│  □ Resource limits are enforced                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Appendix A: Glossary

| Term              | Definition                                                     |
| ----------------- | -------------------------------------------------------------- |
| **Agent**         | A single instance of the mesh agent software                   |
| **AgentID**       | Unique 16-byte identifier for an agent                         |
| **Exit**          | An agent that opens real TCP connections to destinations       |
| **Forward Table** | Mapping of incoming streams to outgoing destinations           |
| **Half-close**    | Closing one direction of a stream while keeping the other open |
| **Ingress**       | An agent that accepts client connections (e.g., SOCKS5)        |
| **LPM**           | Longest Prefix Match (routing lookup algorithm)                |
| **Peer**          | Another agent connected via a transport                        |
| **Stream**        | A bidirectional virtual TCP connection                         |
| **Transit**       | An agent that forwards streams without local endpoints         |
| **Transport**     | The underlying protocol (QUIC, HTTP/2, WebSocket)              |

---

## Appendix B: Quick Reference

### Frame Types

| Code | Name            | Description         |
| ---- | --------------- | ------------------- |
| 0x01 | STREAM_OPEN     | Open virtual stream |
| 0x02 | STREAM_OPEN_ACK | Stream opened       |
| 0x03 | STREAM_OPEN_ERR | Stream failed       |
| 0x04 | STREAM_DATA     | Payload data        |
| 0x05 | STREAM_CLOSE    | Graceful close      |
| 0x06 | STREAM_RESET    | Abort stream        |
| 0x10 | ROUTE_ADVERTISE | Announce routes     |
| 0x11 | ROUTE_WITHDRAW  | Remove routes       |
| 0x20 | PEER_HELLO      | Handshake           |
| 0x21 | PEER_HELLO_ACK  | Handshake response  |
| 0x22 | KEEPALIVE       | Liveness probe      |
| 0x23 | KEEPALIVE_ACK   | Liveness response   |

### Error Codes

| Code | Name                | Description                    |
| ---- | ------------------- | ------------------------------ |
| 1    | NO_ROUTE            | No route to destination        |
| 2    | CONNECTION_REFUSED  | Target refused connection      |
| 3    | CONNECTION_TIMEOUT  | Connection attempt timed out   |
| 4    | TTL_EXCEEDED        | TTL reached zero               |
| 5    | HOST_UNREACHABLE    | Cannot reach target host       |
| 6    | NETWORK_UNREACHABLE | Cannot reach target network    |
| 7    | DNS_ERROR           | Domain name resolution failed  |
| 8    | EXIT_DISABLED       | Exit functionality not enabled |
| 9    | RESOURCE_LIMIT      | Too many streams               |

### Default Timing

| Parameter                | Default |
| ------------------------ | ------- |
| Keepalive idle threshold | 30s     |
| Keepalive timeout        | 90s     |
| Route TTL                | 5m      |
| Advertise interval       | 2m      |
| Reconnect initial delay  | 1s      |
| Reconnect max delay      | 60s     |
| Stream open timeout      | 30s     |
| Handshake timeout        | 10s     |

---

_End of Document_
