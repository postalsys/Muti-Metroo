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

# Certificate Management
./build/muti-metroo cert ca -n "My CA"           # Generate CA certificate
./build/muti-metroo cert agent -n "agent-1"      # Generate agent/peer certificate
./build/muti-metroo cert client -n "client-1"    # Generate client certificate
./build/muti-metroo cert info ./certs/ca.crt     # Display certificate info

# Run
make run                      # Run agent with ./config.yaml
./build/muti-metroo init -d ./data           # Initialize new agent
./build/muti-metroo run -c ./config.yaml     # Run with config file

# Docker (preferred for building and testing)
docker compose build                          # Build all images
docker compose up -d agent1 agent2 agent3     # Start 3-agent mesh testbed
docker compose logs -f agent1                 # Follow logs for agent1
docker compose down                           # Stop all containers
docker compose run test                       # Run tests in container
```

## Development Environment Guidelines

**Always use Docker for building and testing** unless explicitly requested otherwise. This ensures consistent environments and avoids host system dependencies.

```bash
# Preferred: Docker-based development
docker compose build                          # Build images
docker compose up -d agent1 agent2 agent3     # Start testbed
docker compose run test                       # Run tests

# Test endpoints from host
curl http://localhost:8081/health             # Agent1 health
curl http://localhost:8082/health             # Agent2 health
curl http://localhost:8083/health             # Agent3 health
```

**Exception: SSH client testing** - Run SSH client on the host machine, not in Docker containers. This tests the actual SOCKS5 proxy path correctly.

```bash
# SSH via SOCKS5 proxy (run from HOST, not container)
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@target-host
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
| `certutil` | TLS certificate generation and management - CA, server, client, peer certs |
| `rpc` | Remote Procedure Call - shell command execution, whitelist, authentication |
| `health` | Health check HTTP server, Prometheus metrics, remote agent metrics/RPC |
| `control` | Unix socket control interface for CLI status commands |
| `wizard` | Interactive setup wizard with certificate generation |

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
- `health`: Health check HTTP server
- `control`: Unix socket for CLI commands
- `rpc`: Remote command execution (disabled by default)

## Key Implementation Details

- **Frame size**: Max 16 KB payload for write fairness
- **Buffer size**: 256 KB per stream default
- **Timeouts**: Handshake 10s, Stream open 30s, Idle stream 5m
- **Keepalive**: Every 30s idle, 90s timeout
- **Reconnection**: Exponential backoff 1s→60s with 20% jitter
- **Protocol version**: 0x01 (in PEER_HELLO)

## Limits and Performance Characteristics

### Configurable Limits

| Parameter | Config Key | Default | Valid Range | Description |
|-----------|------------|---------|-------------|-------------|
| Max Hops | `routing.max_hops` | 16 | 1-255 | Maximum hops for route advertisements |
| Route TTL | `routing.route_ttl` | 5m | - | Time before routes expire without refresh |
| Advertise Interval | `routing.advertise_interval` | 30s | - | Route advertisement frequency |
| Stream Open Timeout | `limits.stream_open_timeout` | 30s | - | Total round-trip time for STREAM_OPEN |
| Buffer Size | `limits.buffer_size` | 256 KB | - | Per-stream buffer at each hop |
| Max Streams/Peer | `limits.max_streams_per_peer` | 1000 | - | Concurrent streams per peer connection |
| Max Total Streams | `limits.max_streams_total` | 10000 | - | Total concurrent streams across all peers |
| Max Pending Opens | `limits.max_pending_opens` | 100 | - | Pending stream open requests |
| Idle Threshold | `connections.idle_threshold` | 5m | - | Keepalive interval for idle connections |
| Connection Timeout | `connections.timeout` | 90s | - | Disconnect after this keepalive timeout |

### Protocol Constants (Non-configurable)

| Constant | Value | Description |
|----------|-------|-------------|
| Max Frame Payload | 16 KB | Maximum payload per frame |
| Max Frame Size | 16 KB + 14 bytes | Payload + header |
| Header Size | 14 bytes | Frame header (type, flags, stream ID, length) |
| Protocol Version | 0x01 | Current wire protocol version |
| Control Stream ID | 0 | Reserved for control channel |

### Proxy Chain Practical Limits

**Important**: `max_hops` only limits route advertisement propagation, NOT stream path length. Stream paths are limited by the 30-second open timeout.

| Use Case | Recommended Max Hops | Limiting Factor |
|----------|---------------------|-----------------|
| Interactive SSH | 8-12 hops | Latency (~5-50ms per hop) |
| Video Streaming | 6-10 hops | Buffering (256KB × hops) |
| Bulk Transfer | 12-16 hops | Throughput (16KB chunks) |
| High-latency WAN | 4-6 hops | 30s stream open timeout |

**Per-hop overhead:**
- Latency: +1-5ms (LAN), +50-200ms (WAN)
- Memory: +256KB buffer per active stream
- CPU: Frame decode/encode at each relay

### Route Selection Algorithm

Routes are selected using **longest-prefix-match** with metric tiebreaker:

1. Filter routes where CIDR contains destination IP
2. Select route with longest prefix length (most specific wins)
3. If tied, select lowest metric (hop count)

Example with routes from different agents:
- `1.2.3.4/32` (metric 3) - Most specific, wins for 1.2.3.4
- `1.2.3.0/24` (metric 2) - Wins for 1.2.3.5-1.2.3.255
- `0.0.0.0/0` (metric 1) - Default route, wins for everything else

### Topology Support

The flood-based routing supports arbitrary mesh topologies:
- **Linear chains**: A→B→C→D
- **Tree structures**: A→B→C and A→B→D (branches from B)
- **Full mesh**: Any agent can connect to any other
- **Redundant paths**: Multiple paths to same destination (lowest metric wins)

Loop prevention uses `SeenBy` lists in route advertisements - each agent tracks which peers have already seen an advertisement.

## Remote Procedure Call (RPC)

The RPC feature allows executing shell commands on remote agents for maintenance and diagnostics.

### Configuration

```yaml
rpc:
  enabled: true                    # Enable/disable RPC
  whitelist:                       # Allowed commands (empty = none, ["*"] = all)
    - whoami
    - hostname
    - ip
  password_hash: "sha256..."       # SHA-256 hash of RPC password
  timeout: 60s                     # Default command timeout
```

### Security Features

1. **Command Whitelist**: Only commands in the whitelist can be executed
   - Empty list = no commands allowed (default)
   - `["*"]` = all commands allowed (testing only!)
   - Specific commands: `["whoami", "hostname", "ip"]`

2. **Password Authentication**: RPC requests must include the correct password
   - Password is hashed with SHA-256 and stored in config
   - Generate hash: `echo -n "password" | sha256sum`
   - Setup wizard can generate the hash automatically

### HTTP API

**Endpoint**: `POST /agents/{agent-id}/rpc`

**Request**:
```json
{
  "password": "your-rpc-password",
  "command": "whoami",
  "args": ["-a"],
  "stdin": "base64-encoded-input",
  "timeout": 30
}
```

**Response**:
```json
{
  "exit_code": 0,
  "stdout": "base64-encoded-output",
  "stderr": "base64-encoded-errors",
  "error": ""
}
```

### Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `muti_metroo_rpc_calls_total` | Counter | `result`, `command` | Total RPC calls by result |
| `muti_metroo_rpc_call_duration_seconds` | Histogram | `command` | RPC call duration |
| `muti_metroo_rpc_bytes_received_total` | Counter | - | Bytes received in requests |
| `muti_metroo_rpc_bytes_sent_total` | Counter | - | Bytes sent in responses |

Result labels: `success`, `failed`, `rejected`, `auth_failed`, `error`

### Package Structure

| Package | Purpose |
|---------|---------|
| `rpc` | RPC executor, request/response types, metrics, chunking for large payloads |

### Implementation Details

- **Max stdin size**: 1 MB
- **Max output size**: 4 MB (stdout + stderr each)
- **Chunking**: Large payloads split into 14 KB chunks with gzip compression
- **Timeout**: Default 60s, configurable per-request
