# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**Note:** The current year is 2026. Keep documentation and references up to date accordingly.

## Project Overview

Muti Metroo is a userspace mesh networking agent written in Go that creates virtual TCP tunnels across heterogeneous transport layers. It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

**Key capabilities:**

- End-to-end encryption: X25519 key exchange + ChaCha20-Poly1305 (transit cannot decrypt)
- Multiple transport layers: QUIC/TLS 1.3, HTTP/2, and WebSocket
- SOCKS5 proxy ingress with optional authentication
- CIDR-based exit routing (DNS resolved at ingress agent)
- Multi-hop path routing via flood-based route propagation
- Stream multiplexing with half-close support

## Documentation

The project documentation is built with Docusaurus and lives in the `docs/` folder. The documentation is deployed at https://mutimetroo.com.

**Audience distinction:**

- **Docusaurus (`docs/`)**: Public user manual for operators deploying and using Muti Metroo. Focus on installation, configuration, CLI usage, and operational guides. **Do NOT include implementation internals** such as frame protocol details, stream state machines, wire formats, or code architecture.
- **Architecture.md**: Developer documentation for contributors. Contains protocol internals, frame formats, package structure, and implementation details.

**URL Structure:** The public website uses `/` as root, not `/docs`. Source files in `docs/docs/` map to URLs without the `docs` prefix:

- `docs/docs/cli/overview.md` -> https://mutimetroo.com/cli/overview
- `docs/docs/security/red-team-operations.md` -> https://mutimetroo.com/security/red-team-operations

**Important:** When adding or modifying features, the documentation must be updated accordingly:

1. **New features**: Add documentation pages under `docs/docs/` in the appropriate category
2. **CLI changes**: Update the CLI reference in `docs/docs/cli/`
3. **API changes**: Update the HTTP API reference in `docs/docs/api/`
4. **Configuration changes**: Update `docs/docs/configuration/`
5. **Protocol/internals changes**: Update `Architecture.md` only (not Docusaurus)

To work with documentation locally:

```bash
cd docs
npm install        # Install dependencies (first time only)
npm start          # Start dev server at http://localhost:3000
npm run build      # Build for production
```

### Deploying Documentation to Production

Documentation is hosted at https://mutimetroo.com on `srv-04.emailengine.dev`.

**Manual deployment** (for docs-only changes without a release):

```bash
cd docs
npm run build
rsync -avz --delete --exclude 'downloads/' build/ andris@srv-04.emailengine.dev:/var/www/muti-metroo/
```

**Full release deployment**: The `scripts/release.sh` script automatically builds and deploys documentation as part of the release process. It also uploads binaries to `/var/www/muti-metroo/downloads/`.

**Server details:**

- Server: `srv-04.emailengine.dev`
- SSH user: `andris`
- Web root: `/var/www/muti-metroo`
- Downloads: `/var/www/muti-metroo/downloads/latest/` and `/var/www/muti-metroo/downloads/v{version}/`

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

# Password Hash Generation (for SOCKS5, shell, file transfer authentication)
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

- **Ingress**: SOCKS5 listener, DNS resolution, initiates virtual streams, performs route lookup
- **Transit**: Forwards streams between peers, participates in route flooding
- **Exit**: Opens real TCP connections, advertises CIDR routes

### Package Structure (`internal/`)

| Package        | Purpose                                                                                     |
| -------------- | ------------------------------------------------------------------------------------------- |
| `agent`        | Main orchestrator - initializes components, dispatches frames, manages lifecycle            |
| `certutil`     | TLS certificate generation and management - CA, server, client, peer certs                  |
| `chaos`        | Chaos testing utilities - fault injection, ChaosMonkey for resilience testing               |
| `config`       | YAML config parsing with env var substitution (`${VAR:-default}`)                           |
| `crypto`       | End-to-end encryption - X25519 key exchange, ChaCha20-Poly1305, session key derivation      |
| `exit`         | Exit node handler - TCP dial, route-based access control, E2E decryption                    |
| `filetransfer` | Streaming file/directory transfer with tar, gzip, and permission preservation               |
| `flood`        | Route propagation via flooding with loop prevention and seen-cache                          |
| `health`       | Health check HTTP server, remote agent status, pprof, dashboard                             |
| `identity`     | 128-bit AgentID generation, X25519 keypair storage for E2E encryption                       |
| `integration`  | Integration tests for multi-agent mesh scenarios                                            |
| `loadtest`     | Load testing utilities - stream throughput, route table, connection churn                   |
| `logging`      | Structured logging with slog - text/JSON formats, standard attribute keys                   |
| `peer`         | Peer connection lifecycle - handshake, keepalive, reconnection with backoff                 |
| `protocol`     | Binary frame protocol - 14-byte header, encode/decode for all frame types                   |
| `recovery`     | Panic recovery utilities for goroutines with logging and callbacks                          |
| `routing`      | Route table with longest-prefix match, route manager with subscription system               |
| `service`      | Cross-platform service management - systemd (Linux), launchd (macOS), Windows Service       |
| `shell`        | Remote shell - interactive (PTY) and streaming command execution, whitelist, authentication |
| `socks5`       | SOCKS5 server with no-auth and username/password methods                                    |
| `stream`       | Stream state machine (Opening->Open->HalfClosed->Closed), buffered I/O                      |
| `sysinfo`      | System information collection for node info advertisements                                  |
| `transport`    | Transport abstraction with QUIC, HTTP/2, and WebSocket implementations                      |
| `webui`        | Embedded web dashboard with metro map visualization                                         |
| `wizard`       | Interactive setup wizard with certificate generation                                        |

### Frame Flow

1. Client connects to SOCKS5 proxy on ingress agent
2. Agent looks up route via longest-prefix match
3. `STREAM_OPEN` frame sent through path to exit agent (includes ephemeral public key)
4. Exit agent opens real TCP connection, performs key exchange, sends `STREAM_OPEN_ACK` (includes ephemeral public key)
5. Both sides derive shared session key via X25519 ECDH
6. `STREAM_DATA` frames relay bidirectionally (encrypted with ChaCha20-Poly1305, transit cannot decrypt)
7. Half-close via `FIN_WRITE`/`FIN_READ` flags, full close via `STREAM_CLOSE`

### Stream ID Allocation

- Connection initiator (dialer): ODD IDs (1, 3, 5...)
- Connection acceptor (listener): EVEN IDs (2, 4, 6...)
- StreamID 0 reserved for control channel

## Configuration

Example config in `configs/example.yaml`. Key sections:

- `agent`: ID, data_dir, display_name, logging
- `tls`: Global TLS settings (CA, cert, key, mTLS)
- `protocol`: Protocol identifiers for OPSEC customization (ALPN, HTTP header, WS subprotocol)
- `listeners`: Transport listeners (QUIC on :4433)
- `peers`: Outbound peer connections with TLS config
- `socks5`: Ingress proxy settings
- `exit`: Exit node routes (DNS config reserved for future use)
- `routing`: Advertisement intervals, node info interval, TTL, max hops
- `limits`: Stream limits and buffer sizes
- `http`: HTTP API server with granular endpoint control (health, metrics, dashboard, remote APIs, CLI)
- `shell`: Remote shell access (disabled by default)
- `file_transfer`: File upload/download (disabled by default)
- `management`: Management key encryption for topology compartmentalization

### Protocol Identifiers (OPSEC)

The `protocol` section allows customizing identifiers that appear in network traffic:

```yaml
protocol:
  alpn: "muti-metroo/1" # ALPN for QUIC/TLS (empty to disable)
  http_header: "X-Muti-Metroo-Protocol" # HTTP/2 header (empty to disable)
  ws_subprotocol: "muti-metroo/1" # WebSocket subprotocol (empty to disable)
```

For stealth deployments, set all values to empty strings to disable custom identifiers.

### HTTP Endpoint Control

The `http` section supports granular endpoint toggling:

```yaml
http:
  enabled: true
  address: ":8080"
  minimal: false # When true, only /health, /healthz, /ready are enabled
  pprof: false # /debug/pprof/* endpoints (disable in production)
  dashboard: true # /ui/*, /api/* endpoints
  remote_api: true # /agents/* endpoints
```

Disabled endpoints return HTTP 404 and log access attempts at debug level.

## Key Implementation Details

- **Frame size**: Max 16 KB payload for write fairness
- **Buffer size**: 256 KB per stream default
- **Timeouts**: Handshake 10s, Stream open 30s, Idle stream 5m
- **Keepalive**: Every 30s idle, 90s timeout
- **Reconnection**: Exponential backoff 1s→60s with 20% jitter
- **Protocol version**: 0x01 (in PEER_HELLO)

## Limits and Performance Characteristics

### Configurable Limits

| Parameter           | Config Key                    | Default | Valid Range | Description                               |
| ------------------- | ----------------------------- | ------- | ----------- | ----------------------------------------- |
| Max Hops            | `routing.max_hops`            | 16      | 1-255       | Maximum hops for route advertisements     |
| Route TTL           | `routing.route_ttl`           | 5m      | -           | Time before routes expire without refresh |
| Advertise Interval  | `routing.advertise_interval`  | 2m      | -           | Route advertisement frequency             |
| Node Info Interval  | `routing.node_info_interval`  | 2m      | -           | Node info advertisement frequency         |
| Stream Open Timeout | `limits.stream_open_timeout`  | 30s     | -           | Total round-trip time for STREAM_OPEN     |
| Buffer Size         | `limits.buffer_size`          | 256 KB  | -           | Per-stream buffer at each hop             |
| Max Streams/Peer    | `limits.max_streams_per_peer` | 1000    | -           | Concurrent streams per peer connection    |
| Max Total Streams   | `limits.max_streams_total`    | 10000   | -           | Total concurrent streams across all peers |
| Max Pending Opens   | `limits.max_pending_opens`    | 100     | -           | Pending stream open requests              |
| Idle Threshold      | `connections.idle_threshold`  | 5m      | -           | Keepalive interval for idle connections   |
| Connection Timeout  | `connections.timeout`         | 90s     | -           | Disconnect after this keepalive timeout   |

### Protocol Constants (Non-configurable)

| Constant          | Value            | Description                                   |
| ----------------- | ---------------- | --------------------------------------------- |
| Max Frame Payload | 16 KB            | Maximum payload per frame                     |
| Max Frame Size    | 16 KB + 14 bytes | Payload + header                              |
| Header Size       | 14 bytes         | Frame header (type, flags, stream ID, length) |
| Protocol Version  | 0x01             | Current wire protocol version                 |
| Control Stream ID | 0                | Reserved for control channel                  |

### Proxy Chain Practical Limits

**Important**: `max_hops` only limits route advertisement propagation, NOT stream path length. Stream paths are limited by the 30-second open timeout.

| Use Case         | Recommended Max Hops | Limiting Factor           |
| ---------------- | -------------------- | ------------------------- |
| Interactive SSH  | 8-12 hops            | Latency (~5-50ms per hop) |
| Video Streaming  | 6-10 hops            | Buffering (256KB × hops)  |
| Bulk Transfer    | 12-16 hops           | Throughput (16KB chunks)  |
| High-latency WAN | 4-6 hops             | 30s stream open timeout   |

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
{ "status": "triggered", "message": "route advertisement triggered" }
```

**Programmatic access**: The agent exposes `TriggerRouteAdvertise()` method which can be called internally when routes change.

## HTTP API Endpoints

The health server exposes several HTTP endpoints for monitoring, management, and distributed operations.

### Health & Monitoring

| Endpoint   | Method | Description                                                      |
| ---------- | ------ | ---------------------------------------------------------------- |
| `/health`  | GET    | Basic health check, returns "OK"                                 |
| `/healthz` | GET    | Detailed health with JSON stats (peer count, stream count, etc.) |
| `/ready`   | GET    | Readiness probe for Kubernetes                                   |

### Web Dashboard

| Endpoint         | Method | Description                                           |
| ---------------- | ------ | ----------------------------------------------------- |
| `/ui/`           | GET    | Embedded web dashboard with metro map visualization   |
| `/api/topology`  | GET    | Topology data for metro map (agents and connections)  |
| `/api/dashboard` | GET    | Dashboard overview (agent info, stats, peers, routes) |
| `/api/nodes`     | GET    | Detailed node info listing for all known agents       |

### Distributed Status

| Endpoint                           | Method | Description                            |
| ---------------------------------- | ------ | -------------------------------------- |
| `/agents`                          | GET    | List all known agents in the mesh      |
| `/agents/{agent-id}`               | GET    | Get status from specific agent         |
| `/agents/{agent-id}/routes`        | GET    | Get route table from specific agent    |
| `/agents/{agent-id}/peers`         | GET    | Get peer list from specific agent      |
| `/agents/{agent-id}/shell`         | GET    | WebSocket shell access on remote agent |
| `/agents/{agent-id}/file/upload`   | POST   | Upload file to remote agent            |
| `/agents/{agent-id}/file/download` | POST   | Download file from remote agent        |

### Management

| Endpoint            | Method | Description                           |
| ------------------- | ------ | ------------------------------------- |
| `/routes/advertise` | POST   | Trigger immediate route advertisement |

### Debugging (pprof)

| Endpoint               | Method | Description                    |
| ---------------------- | ------ | ------------------------------ |
| `/debug/pprof/`        | GET    | pprof index                    |
| `/debug/pprof/cmdline` | GET    | Running program's command line |
| `/debug/pprof/profile` | GET    | CPU profile                    |
| `/debug/pprof/symbol`  | GET    | Symbol lookup                  |
| `/debug/pprof/trace`   | GET    | Execution trace                |

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

## Remote Shell

The shell feature allows executing commands on remote agents with both streaming (default) and interactive (PTY) modes.

### CLI Usage

```bash
# Streaming mode (default) - for simple commands and continuous output
muti-metroo shell <target-agent-id> [command] [args...]

# Examples:
muti-metroo shell abc123def456 whoami
muti-metroo shell abc123def456 journalctl -u muti-metroo -f
muti-metroo shell abc123def456 tail -f /var/log/syslog

# Interactive mode (--tty) - for programs requiring a terminal
muti-metroo shell --tty abc123def456 bash
muti-metroo shell --tty abc123def456 vim /etc/config.yaml
muti-metroo shell --tty abc123def456 htop

# Via a different agent's health server
muti-metroo shell -a 192.168.1.10:8080 --tty abc123def456 top

# With password authentication
muti-metroo shell -p mysecret abc123def456 whoami
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent health server address |
| `--password` | `-p` | | Shell password for authentication |
| `--timeout` | `-t` | `0` | Session timeout in seconds (0 = no timeout) |
| `--tty` | | | Interactive mode with PTY (for vim, bash, htop, etc.) |

### Configuration

```yaml
shell:
  enabled: false # Disabled by default (security)
  whitelist: [] # Commands allowed (empty = none, ["*"] = all)
  password_hash: "" # bcrypt hash of shell password
  timeout: 0s # Optional command timeout (0 = no timeout)
  max_sessions: 10 # Max concurrent sessions
```

### Security Features

1. **Command Whitelist**: Only commands in the whitelist can be executed

   - Empty list = no commands allowed (default)
   - `["*"]` = all commands allowed (testing only!)
   - Specific commands: `["bash", "vim", "whoami"]`

2. **Password Authentication**: Shell requests must include the correct password
   - Password is hashed with bcrypt and stored in config
   - Generate hash: `muti-metroo hash --cost 12`
   - Setup wizard can generate the hash automatically

### Modes

- **Streaming Mode** (default): Non-PTY mode for simple commands and continuous output
- **Interactive Mode** (`--tty`): Allocates a PTY for full terminal support (vim, htop, bash)

### Platform Support

| Platform | Interactive (PTY) | Streaming |
| -------- | ----------------- | --------- |
| Linux    | Yes               | Yes       |
| macOS    | Yes               | Yes       |
| Windows  | Yes (ConPTY)      | Yes       |

### Prometheus Metrics

| Metric                               | Type      | Labels           | Description       |
| ------------------------------------ | --------- | ---------------- | ----------------- |
| `muti_metroo_shell_sessions_active`  | Gauge     | -                | Active sessions   |
| `muti_metroo_shell_sessions_total`   | Counter   | `type`, `result` | Total sessions    |
| `muti_metroo_shell_duration_seconds` | Histogram | -                | Session duration  |
| `muti_metroo_shell_bytes_total`      | Counter   | `direction`      | Bytes transferred |

Type labels: `stream`, `interactive`
Result labels: `success`, `error`, `timeout`, `rejected`
Direction labels: `stdin`, `stdout`, `stderr`

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
  enabled: true # Enable/disable file transfer
  max_file_size: 0 # Max file size in bytes (0 = unlimited)
  allowed_paths: # Works like shell whitelist:
    - /tmp # - Empty [] = no paths allowed
    - /data/** # - ["*"] = all paths allowed
    - /home/*/uploads # - Supports glob patterns
  password_hash: "bcrypt..." # bcrypt hash of password (optional)
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

## Design Decisions

### No Prometheus Metrics

Muti Metroo intentionally does not include Prometheus metrics functionality. In red team exercises:

- Agents are deployed ad hoc and are short-lived (lasting only for the duration of the exercise)
- Setting up monitoring or alerting infrastructure is not practical or necessary
- Operational simplicity is prioritized over observability

**Do not add metrics functionality to this codebase.**
