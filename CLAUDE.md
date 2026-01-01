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

## Documentation

The project documentation is built with Docusaurus and lives in the `docs/` folder. The documentation is deployed at https://muti-metroo.postalsys.com.

**Important:** When adding or modifying features, the documentation must be updated accordingly:

1. **New features**: Add documentation pages under `docs/docs/` in the appropriate category
2. **CLI changes**: Update the CLI reference in `docs/docs/cli/`
3. **API changes**: Update the HTTP API reference in `docs/docs/api/`
4. **Configuration changes**: Update `docs/docs/getting-started/configuration.md`
5. **Protocol changes**: Update `docs/docs/protocol/`

To work with documentation locally:

```bash
cd docs
npm install        # Install dependencies (first time only)
npm start          # Start dev server at http://localhost:3000
npm run build      # Build for production
```

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

# Password Hash Generation (for SOCKS5, RPC, file transfer authentication)
./build/muti-metroo hash                         # Interactive prompt (recommended)
./build/muti-metroo hash "password"              # From argument
./build/muti-metroo hash --cost 12               # Custom cost factor

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

## Code Style Guidelines

- **ASCII only**: Do not use emojis or non-ASCII characters in code, comments, documentation, commit messages, or any other text files. Stick with ASCII characters only.

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
| `certutil` | TLS certificate generation and management - CA, server, client, peer certs |
| `chaos` | Chaos testing utilities - fault injection, ChaosMonkey for resilience testing |
| `config` | YAML config parsing with env var substitution (`${VAR:-default}`) |
| `control` | Unix socket control interface for CLI status commands |
| `exit` | Exit node handler - TCP dial, DNS resolution, route-based access control |
| `filetransfer` | Streaming file/directory transfer with tar, gzip, and permission preservation |
| `flood` | Route propagation via flooding with loop prevention and seen-cache |
| `health` | Health check HTTP server, Prometheus metrics, remote agent metrics/RPC, pprof, dashboard |
| `identity` | 128-bit AgentID generation and persistence |
| `integration` | Integration tests for multi-agent mesh scenarios |
| `loadtest` | Load testing utilities - stream throughput, route table, connection churn |
| `logging` | Structured logging with slog - text/JSON formats, standard attribute keys |
| `metrics` | Prometheus metrics - peers, streams, routing, SOCKS5, exit, protocol stats |
| `peer` | Peer connection lifecycle - handshake, keepalive, reconnection with backoff |
| `protocol` | Binary frame protocol - 14-byte header, encode/decode for all frame types |
| `recovery` | Panic recovery utilities for goroutines with logging and callbacks |
| `routing` | Route table with longest-prefix match, route manager with subscription system |
| `rpc` | Remote Procedure Call - shell command execution, whitelist, authentication |
| `service` | Cross-platform service management - systemd (Linux), launchd (macOS), Windows Service |
| `socks5` | SOCKS5 server with no-auth and username/password methods |
| `stream` | Stream state machine (Opening->Open->HalfClosed->Closed), buffered I/O |
| `sysinfo` | System information collection for node info advertisements |
| `transport` | Transport abstraction with QUIC, HTTP/2, and WebSocket implementations |
| `webui` | Embedded web dashboard with metro map visualization |
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
- `agent`: ID, data_dir, display_name, logging
- `listeners`: Transport listeners (QUIC on :4433)
- `peers`: Outbound peer connections with TLS config
- `socks5`: Ingress proxy settings
- `exit`: Exit node routes and DNS settings
- `routing`: Advertisement intervals, node info interval, TTL, max hops
- `limits`: Stream limits and buffer sizes
- `http`: HTTP API server (health, metrics, dashboard, remote agent APIs)
- `control`: Unix socket for CLI commands
- `rpc`: Remote command execution (disabled by default)
- `file_transfer`: File upload/download (disabled by default)

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
| Advertise Interval | `routing.advertise_interval` | 2m | - | Route advertisement frequency |
| Node Info Interval | `routing.node_info_interval` | 2m | - | Node info advertisement frequency |
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

### Triggering Immediate Route Advertisement

By default, routes are advertised periodically based on `advertise_interval` (default 2 minutes). For faster route propagation after configuration changes, you can trigger immediate advertisement via the HTTP API:

```bash
# Trigger immediate route advertisement on local agent
curl -X POST http://localhost:8080/routes/advertise
```

Response:
```json
{"status": "triggered", "message": "route advertisement triggered"}
```

**Programmatic access**: The agent exposes `TriggerRouteAdvertise()` method which can be called internally when routes change.

## HTTP API Endpoints

The health server exposes several HTTP endpoints for monitoring, management, and distributed operations.

### Health & Monitoring

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Basic health check, returns "OK" |
| `/healthz` | GET | Detailed health with JSON stats (peer count, stream count, etc.) |
| `/ready` | GET | Readiness probe for Kubernetes |
| `/metrics` | GET | Local Prometheus metrics |

### Web Dashboard

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ui/` | GET | Embedded web dashboard with metro map visualization |
| `/api/topology` | GET | Topology data for metro map (agents and connections) |
| `/api/dashboard` | GET | Dashboard overview (agent info, stats, peers, routes) |
| `/api/nodes` | GET | Detailed node info listing for all known agents |

### Distributed Metrics & Status

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/metrics/{agent-id}` | GET | Fetch Prometheus metrics from remote agent via control channel |
| `/agents` | GET | List all known agents in the mesh |
| `/agents/{agent-id}` | GET | Get status from specific agent |
| `/agents/{agent-id}/routes` | GET | Get route table from specific agent |
| `/agents/{agent-id}/peers` | GET | Get peer list from specific agent |
| `/agents/{agent-id}/rpc` | POST | Execute RPC command on remote agent |
| `/agents/{agent-id}/file/upload` | POST | Upload file to remote agent |
| `/agents/{agent-id}/file/download` | POST | Download file from remote agent |

### Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/routes/advertise` | POST | Trigger immediate route advertisement |

### Debugging (pprof)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/debug/pprof/` | GET | pprof index |
| `/debug/pprof/cmdline` | GET | Running program's command line |
| `/debug/pprof/profile` | GET | CPU profile |
| `/debug/pprof/symbol` | GET | Symbol lookup |
| `/debug/pprof/trace` | GET | Execution trace |

## Service Installation

Muti Metroo can be installed as a system service on Linux (systemd), macOS (launchd), and Windows.

### Commands

```bash
# Install as system service
sudo muti-metroo service install -c /path/to/config.yaml

# Check service status
muti-metroo service status

# Uninstall service
sudo muti-metroo service uninstall
```

### Linux (systemd)

The installer creates a systemd unit file at `/etc/systemd/system/muti-metroo.service`:

```bash
# After installation
sudo systemctl start muti-metroo
sudo systemctl enable muti-metroo
sudo journalctl -u muti-metroo -f
```

### macOS (launchd)

The installer creates a launchd plist at `/Library/LaunchDaemons/com.muti-metroo.plist`:

```bash
# After installation
sudo launchctl start com.muti-metroo
tail -f /var/log/muti-metroo.out.log
```

### Windows

On Windows, the agent registers as a Windows Service and can be managed via:

```powershell
# Start/stop via Services console or:
sc start muti-metroo
sc stop muti-metroo
```

**Note**: Service installation requires root/administrator privileges.

## Remote Procedure Call (RPC)

The RPC feature allows executing shell commands on remote agents for maintenance and diagnostics.

### CLI Usage

```bash
# Execute command on remote agent (via localhost:8080)
muti-metroo rpc <target-agent-id> <command> [args...]

# Examples:
muti-metroo rpc abc123def456 whoami
muti-metroo rpc abc123def456 ls -la /tmp
muti-metroo rpc abc123def456 ip addr show

# Via a different agent's health server
muti-metroo rpc -a 192.168.1.10:8080 abc123def456 hostname

# With password authentication
muti-metroo rpc -p mysecret abc123def456 whoami

# With custom timeout (seconds)
muti-metroo rpc -t 120 abc123def456 long-running-script.sh

# Pipe stdin to remote command
echo "hello world" | muti-metroo rpc abc123def456 cat
cat file.txt | muti-metroo rpc abc123def456 wc -l
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent health server address |
| `--password` | `-p` | | RPC password for authentication |
| `--timeout` | `-t` | `60` | Command timeout in seconds |

The CLI forwards stdout/stderr and exits with the remote command's exit code.

### Configuration

```yaml
rpc:
  enabled: true                    # Enable/disable RPC
  whitelist:                       # Allowed commands (empty = none, ["*"] = all)
    - whoami
    - hostname
    - ip
  password_hash: "$2a$10$..."      # bcrypt hash of RPC password
  timeout: 60s                     # Default command timeout
```

### Security Features

1. **Command Whitelist**: Only commands in the whitelist can be executed
   - Empty list = no commands allowed (default)
   - `["*"]` = all commands allowed (testing only!)
   - Specific commands: `["whoami", "hostname", "ip"]`

2. **Password Authentication**: RPC requests must include the correct password
   - Password is hashed with bcrypt and stored in config
   - Generate hash: `muti-metroo hash --cost 12`
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

### Implementation Details

- **Max stdin size**: 1 MB
- **Max output size**: 4 MB (stdout + stderr each)
- **Chunking**: Large payloads split into 14 KB chunks with gzip compression
- **Timeout**: Default 60s, configurable per-request

## File Transfer

The file transfer feature allows uploading and downloading files and directories to/from remote agents using streaming transfers.

### CLI Usage

```bash
# Upload a file
muti-metroo upload <target-agent-id> <local-path> <remote-path>

# Upload a directory (auto-detected)
muti-metroo upload abc123def456 ./my-folder /tmp/my-folder

# Download a file
muti-metroo download <target-agent-id> <remote-path> <local-path>

# Download a directory
muti-metroo download abc123def456 /etc/myapp ./myapp-config

# With password authentication
muti-metroo upload -p secret abc123def456 ./data.bin /tmp/data.bin

# Via a different agent's health server
muti-metroo upload -a 192.168.1.10:8080 abc123def456 ./file.txt /tmp/file.txt
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent health server address |
| `--password` | `-p` | | File transfer password |
| `--timeout` | `-t` | `300` | Transfer timeout in seconds |

### Configuration

```yaml
file_transfer:
  enabled: true                    # Enable/disable file transfer
  max_file_size: 0                 # Max file size in bytes (0 = unlimited)
  allowed_paths:                   # Allowed path prefixes (empty = all absolute paths)
    - /tmp
    - /home/user/uploads
  password_hash: "bcrypt..."       # bcrypt hash of password (optional)
```

### HTTP API

**Upload**: `POST /agents/{agent-id}/file/upload`

Content-Type: `multipart/form-data`

Form fields:
- `file`: The file to upload (can be tar archive for directories)
- `path`: Remote destination path (required)
- `password`: Authentication password (optional)
- `directory`: "true" if uploading a directory tar (optional)

Response:
```json
{
  "success": true,
  "bytes_written": 1024,
  "remote_path": "/tmp/myfile.txt"
}
```

**Download**: `POST /agents/{agent-id}/file/download`

Request:
```json
{
  "password": "your-password",
  "path": "/tmp/myfile.txt"
}
```

Response: Binary file data with headers:
- `Content-Type`: `application/octet-stream` (file) or `application/gzip` (directory)
- `Content-Disposition`: Filename
- `X-File-Mode`: File permissions (octal, e.g., "0644")

### Implementation Details

- **Streaming**: Files are streamed in 16KB chunks (no memory limits)
- **Unlimited size**: No inherent file size limit
- **Directories**: Automatically tar/untar with gzip compression
- **Permissions**: File mode is preserved during transfer
- **Authentication**: bcrypt password hashing

## Prometheus Metrics

All metrics are prefixed with `muti_metroo_`. Available at `/metrics` endpoint.

### Connection Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `peers_connected` | Gauge | - | Currently connected peers |
| `peers_total` | Counter | - | Total peer connections established |
| `peer_connections_total` | Counter | `transport`, `direction` | Connections by transport type |
| `peer_disconnects_total` | Counter | `reason` | Disconnections by reason |

### Stream Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `streams_active` | Gauge | - | Currently active streams |
| `streams_opened_total` | Counter | - | Total streams opened |
| `streams_closed_total` | Counter | - | Total streams closed |
| `stream_open_latency_seconds` | Histogram | - | Stream open latency |
| `stream_errors_total` | Counter | `error_type` | Stream errors by type |

### Data Transfer Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `bytes_sent_total` | Counter | `type` | Bytes sent by type |
| `bytes_received_total` | Counter | `type` | Bytes received by type |
| `frames_sent_total` | Counter | `frame_type` | Frames sent by type |
| `frames_received_total` | Counter | `frame_type` | Frames received by type |

### Routing Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `routes_total` | Gauge | - | Routes in routing table |
| `route_advertises_total` | Counter | - | Route advertisements processed |
| `route_withdrawals_total` | Counter | - | Route withdrawals processed |
| `route_flood_latency_seconds` | Histogram | - | Route flood propagation latency |

### SOCKS5 Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `socks5_connections_active` | Gauge | - | Active SOCKS5 connections |
| `socks5_connections_total` | Counter | - | Total SOCKS5 connections |
| `socks5_auth_failures_total` | Counter | - | Authentication failures |
| `socks5_connect_latency_seconds` | Histogram | - | Connect request latency |

### Exit Handler Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `exit_connections_active` | Gauge | - | Active exit connections |
| `exit_connections_total` | Counter | - | Total exit connections |
| `exit_dns_queries_total` | Counter | - | DNS queries performed |
| `exit_dns_latency_seconds` | Histogram | - | DNS query latency |
| `exit_errors_total` | Counter | `error_type` | Exit errors by type |

### Protocol Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `handshake_latency_seconds` | Histogram | - | Peer handshake latency |
| `handshake_errors_total` | Counter | `error_type` | Handshake errors by type |
| `keepalives_sent_total` | Counter | - | Keepalives sent |
| `keepalives_received_total` | Counter | - | Keepalives received |
| `keepalive_rtt_seconds` | Histogram | - | Keepalive round-trip time |
