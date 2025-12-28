# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Muti Metroo is a userspace mesh networking agent written in Go that creates virtual TCP tunnels across heterogeneous transport layers. It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

**Key capabilities:**
- Multiple transport layers: QUIC/TLS 1.3, HTTP/2, and WebSocket
- SOCKS5 proxy ingress with optional authentication
- CIDR-based exit routing with DNS resolution
- Multi-hop path routing via flood-based route propagation
- Stream multiplexing with half-close support

## Build & Development Commands

```bash
# Build
make build                    # Build binary to ./build/muti-metroo

# Testing
make test                     # Run all tests with race detection
make test-coverage            # Generate coverage report to ./coverage/
make test-short               # Run short tests only
go test -v ./internal/...     # Run specific package tests
go test -v -run TestName ./internal/peer/  # Run single test

# Linting & Formatting
make lint                     # Run gofmt and go vet
make fmt                      # Format code

# Development Setup
make generate-certs           # Generate self-signed TLS certs for dev
make init-dev                 # Initialize data directory and agent identity

# Run
make run                      # Run agent with ./config.yaml
./build/muti-metroo init -d ./data           # Initialize new agent
./build/muti-metroo run -c ./config.yaml     # Run with config file
```

## Architecture

### Agent Roles
An agent can serve multiple roles simultaneously:
- **Ingress**: SOCKS5 listener, initiates virtual streams, performs route lookup
- **Transit**: Forwards streams between peers, participates in route flooding
- **Exit**: Opens real TCP connections, advertises CIDR routes, handles DNS

### Package Structure (`internal/`)

| Package | Purpose |
|---------|---------|
| `agent` | Main orchestrator - initializes components, dispatches frames, manages lifecycle |
| `identity` | 128-bit AgentID generation and persistence |
| `config` | YAML config parsing with env var substitution (`${VAR:-default}`) |
| `protocol` | Binary frame protocol - 14-byte header, encode/decode for all frame types |
| `transport` | Transport abstraction with QUIC implementation (TLS helpers, stream allocation) |
| `peer` | Peer connection lifecycle - handshake, keepalive, reconnection with backoff |
| `routing` | Route table with longest-prefix match, route manager with subscription system |
| `flood` | Route propagation via flooding with loop prevention and seen-cache |
| `stream` | Stream state machine (Opening→Open→HalfClosed→Closed), buffered I/O |
| `socks5` | SOCKS5 server with no-auth and username/password methods |
| `exit` | Exit node handler - TCP dial, DNS resolution, route-based access control |

### Frame Flow
1. Client connects to SOCKS5 proxy on ingress agent
2. Agent looks up route via longest-prefix match
3. `STREAM_OPEN` frame sent through path to exit agent
4. Exit agent opens real TCP connection, sends `STREAM_OPEN_ACK`
5. `STREAM_DATA` frames relay bidirectionally through the chain
6. Half-close via `FIN_WRITE`/`FIN_READ` flags, full close via `STREAM_CLOSE`

### Stream ID Allocation
- Connection initiator (dialer): ODD IDs (1, 3, 5...)
- Connection acceptor (listener): EVEN IDs (2, 4, 6...)
- StreamID 0 reserved for control channel

## Configuration

Example config in `configs/example.yaml`. Key sections:
- `agent`: ID, data_dir, logging
- `listeners`: Transport listeners (QUIC on :4433)
- `peers`: Outbound peer connections with TLS config
- `socks5`: Ingress proxy settings
- `exit`: Exit node routes and DNS settings
- `routing`: Advertisement intervals, TTL, max hops
- `limits`: Stream limits and buffer sizes

## Key Implementation Details

- **Frame size**: Max 16 KB payload for write fairness
- **Buffer size**: 256 KB per stream default
- **Timeouts**: Handshake 10s, Stream open 30s, Idle stream 5m
- **Keepalive**: Every 30s idle, 90s timeout
- **Reconnection**: Exponential backoff 1s→60s with 20% jitter
- **Protocol version**: 0x01 (in PEER_HELLO)
