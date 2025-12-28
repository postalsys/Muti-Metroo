# Muti Metroo

A userspace mesh networking agent written in Go that creates virtual TCP tunnels across heterogeneous transport layers. It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing, operating entirely in userspace without requiring root privileges.

## Features

- **Multiple Transport Layers**: QUIC/TLS 1.3, HTTP/2, and WebSocket transports
- **SOCKS5 Proxy Ingress**: Accept client connections with optional username/password authentication
- **CIDR-Based Exit Routing**: Advertise routes and handle DNS resolution at exit nodes
- **Multi-Hop Mesh Routing**: Flood-based route propagation with longest-prefix match
- **Stream Multiplexing**: Multiple virtual streams over a single peer connection with half-close support
- **Automatic Reconnection**: Exponential backoff with jitter for resilient peer connections
- **No Root Required**: Runs entirely in userspace

## Quick Start

### Prerequisites

- Go 1.23 or later
- Make (optional, for convenience commands)

### Build

```bash
# Clone the repository
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4

# Build the binary
make build

# Or without make:
go build -o build/muti-metroo ./cmd/muti-metroo
```

### Interactive Setup (Recommended)

The easiest way to get started is using the interactive setup wizard:

```bash
./build/muti-metroo setup
```

The wizard guides you through:
- **Basic setup**: Data directory and config file location
- **Agent role**: Ingress (SOCKS5 proxy), Transit (relay), or Exit (external network access)
- **Network config**: Transport protocol (QUIC/HTTP2/WebSocket) and listen address
- **TLS certificates**: Generate new, paste existing, or use certificate files
- **Peer connections**: Connect to other mesh agents
- **SOCKS5 settings**: Configure proxy authentication (for ingress nodes)
- **Exit routes**: Define allowed destination networks (for exit nodes)
- **Advanced options**: Logging, health checks, control socket

The wizard generates a complete `config.yaml` and initializes your agent identity.

### Manual Setup

If you prefer manual configuration:

### Initialize Agent

```bash
# Initialize agent identity
./build/muti-metroo init -d ./data
```

### Generate Certificates

Generate fresh TLS certificates for your deployment:

```bash
# Generate CA certificate (valid for 1 year)
./build/muti-metroo cert ca -n "My Mesh CA" -o ./certs

# Generate agent certificates signed by the CA
./build/muti-metroo cert agent -n "agent-a" --dns "node-a.example.com" --ip "192.168.1.10"
./build/muti-metroo cert agent -n "agent-b" --dns "node-b.example.com" --ip "192.168.1.20"

# Generate client certificate (for mTLS)
./build/muti-metroo cert client -n "admin-client"

# View certificate details
./build/muti-metroo cert info ./certs/agent-a.crt
```

Certificate commands:
- `cert ca` - Generate a new Certificate Authority
- `cert agent` - Generate an agent/peer certificate (server + client auth)
- `cert client` - Generate a client-only certificate
- `cert info` - Display certificate information and expiration status

### Run

```bash
# Copy and edit the example configuration
cp configs/example.yaml config.yaml
# Edit config.yaml with your settings

# Run the agent
./build/muti-metroo run -c ./config.yaml

# Or with make:
make run
```

## Configuration

See `configs/example.yaml` for a fully commented configuration file. Key sections:

### Agent Identity

```yaml
agent:
  id: "auto"           # Auto-generate on first run, or specify hex string
  data_dir: "./data"   # Directory for persistent state
  log_level: "info"    # debug, info, warn, error
  log_format: "text"   # text, json
```

### Transport Listeners

```yaml
listeners:
  - transport: quic    # quic, h2, or ws
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### Peer Connections

```yaml
peers:
  - id: "abc123..."           # Expected peer AgentID
    transport: quic
    address: "192.168.1.50:4433"
    tls:
      ca: "./certs/peer-ca.crt"
```

### SOCKS5 Server (Ingress)

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
    users:
      - username: "user1"
        password: "pass1"
```

### Exit Node

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "0.0.0.0/0"      # Default route
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s
```

## Architecture

### Agent Roles

An agent can serve multiple roles simultaneously:

- **Ingress**: SOCKS5 listener, initiates virtual streams, performs route lookup
- **Transit**: Forwards streams between peers, participates in route flooding
- **Exit**: Opens real TCP connections, advertises CIDR routes, handles DNS

### Data Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │     │   Agent A   │     │   Agent B   │     │   Agent C   │
│  (Browser)  │     │  (Ingress)  │     │  (Transit)  │     │   (Exit)    │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │                   │
       │ SOCKS5 CONNECT    │                   │                   │
       │──────────────────>│                   │                   │
       │                   │                   │                   │
       │                   │ Route Lookup      │                   │
       │                   │ (longest match)   │                   │
       │                   │                   │                   │
       │                   │ STREAM_OPEN       │ STREAM_OPEN       │
       │                   │──────────────────>│──────────────────>│
       │                   │                   │                   │
       │                   │                   │                   │ TCP Connect
       │                   │                   │                   │────────────>
       │                   │                   │                   │
       │                   │ STREAM_OPEN_ACK   │ STREAM_OPEN_ACK   │
       │                   │<──────────────────│<──────────────────│
       │                   │                   │                   │
       │ SOCKS5 OK         │                   │                   │
       │<──────────────────│                   │                   │
       │                   │                   │                   │
       │     STREAM_DATA frames (bidirectional relay)              │
       │<═══════════════════════════════════════════════════════════>
```

### Package Structure

| Package | Purpose |
|---------|---------|
| `agent` | Main orchestrator - initializes components, dispatches frames, manages lifecycle |
| `identity` | 128-bit AgentID generation and persistence |
| `config` | YAML config parsing with env var substitution (`${VAR:-default}`) |
| `protocol` | Binary frame protocol - 14-byte header, encode/decode for all frame types |
| `transport` | Transport abstraction with QUIC, H2, and WebSocket implementations |
| `peer` | Peer connection lifecycle - handshake, keepalive, reconnection with backoff |
| `routing` | Route table with longest-prefix match, route manager with subscription system |
| `flood` | Route propagation via flooding with loop prevention and seen-cache |
| `stream` | Stream state machine (Opening->Open->HalfClosed->Closed), buffered I/O |
| `socks5` | SOCKS5 server with no-auth and username/password methods |
| `exit` | Exit node handler - TCP dial, DNS resolution, route-based access control |
| `certutil` | TLS certificate generation and management - CA, server, client, peer certificates |
| `wizard` | Interactive setup wizard with modern terminal UI |
| `health` | Health check HTTP server with pprof endpoints |
| `control` | Unix socket control server for CLI commands |

## Development

### Build Commands

```bash
make build            # Build binary to ./build/muti-metroo
make test             # Run all tests with race detection
make test-coverage    # Generate coverage report
make test-short       # Run short tests only
make lint             # Run gofmt and go vet
make fmt              # Format code
make clean            # Clean build artifacts
make deps             # Download and tidy dependencies
```

### Running Tests

```bash
# All tests
make test

# Specific package
go test -v ./internal/transport/...

# Single test
go test -v -run TestName ./internal/peer/

# Integration tests
go test -v ./internal/integration/...
```

### Docker

```bash
# Build image
docker build -t muti-metroo .

# Run with config
docker run -v $(pwd)/config.yaml:/app/config.yaml \
           -v $(pwd)/data:/app/data \
           -v $(pwd)/certs:/app/certs \
           -p 1080:1080 -p 4433:4433/udp \
           muti-metroo
```

## Protocol Details

- **Frame Size**: Max 16 KB payload
- **Buffer Size**: 256 KB per stream default
- **Timeouts**: Handshake 10s, Stream open 30s, Idle stream 5m
- **Keepalive**: Every 30s idle, 90s timeout
- **Reconnection**: Exponential backoff 1s->60s with 20% jitter
- **Protocol Version**: 0x01

### Frame Types

| Type | Name | Description |
|------|------|-------------|
| 0x01 | PEER_HELLO | Handshake initiation |
| 0x02 | PEER_HELLO_ACK | Handshake acknowledgment |
| 0x10 | STREAM_OPEN | Open a new virtual stream |
| 0x11 | STREAM_OPEN_ACK | Stream opened successfully |
| 0x12 | STREAM_OPEN_ERR | Stream open failed |
| 0x20 | STREAM_DATA | Stream data payload |
| 0x21 | STREAM_CLOSE | Close stream |
| 0x22 | STREAM_RESET | Reset stream with error |
| 0x30 | ROUTE_UPDATE | Route advertisement |
| 0x40 | KEEPALIVE | Connection keepalive |
| 0x41 | KEEPALIVE_ACK | Keepalive acknowledgment |

## License

Proprietary - All rights reserved.
