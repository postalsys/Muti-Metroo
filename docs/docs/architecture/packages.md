---
title: Package Structure
---

# Package Structure

Internal packages in `internal/`:

| Package | Purpose |
|---------|---------|
| `agent` | Main orchestrator - initializes components, dispatches frames |
| `certutil` | TLS certificate generation (CA, server, client, peer) |
| `chaos` | Chaos testing utilities and fault injection |
| `config` | YAML parsing with environment variable substitution |
| `control` | Unix socket control interface for CLI |
| `exit` | Exit node - TCP dial, DNS resolution, route ACL |
| `filetransfer` | Streaming file/directory transfer |
| `flood` | Route flooding with loop prevention |
| `health` | HTTP server - health checks, metrics, RPC, pprof |
| `identity` | 128-bit AgentID generation and persistence |
| `integration` | Integration tests for multi-agent scenarios |
| `loadtest` | Load testing utilities |
| `logging` | Structured logging with slog |
| `metrics` | Prometheus metrics definitions |
| `peer` | Peer lifecycle - handshake, keepalive, reconnection |
| `protocol` | Binary frame protocol encoder/decoder |
| `recovery` | Panic recovery for goroutines |
| `routing` | Route table with longest-prefix match |
| `rpc` | Remote command execution |
| `service` | Cross-platform service management (systemd, Windows) |
| `socks5` | SOCKS5 server implementation |
| `stream` | Stream state machine and I/O buffering |
| `sysinfo` | System information collection |
| `transport` | Transport abstraction (QUIC, HTTP/2, WebSocket) |
| `webui` | Embedded web dashboard |
| `wizard` | Interactive setup wizard |

## Key Interfaces

### Transport

```go
type Transport interface {
    Dial(addr string) (net.Conn, error)
    Listen(addr string) (net.Listener, error)
}
```

### Frame Handler

```go
type FrameHandler interface {
    HandleFrame(peerID AgentID, frame *Frame) error
}
```

### Route Manager

```go
type RouteManager interface {
    AddRoute(route Route) error
    RemoveRoute(cidr string) error
    Lookup(ip net.IP) (*Route, error)
}
```
