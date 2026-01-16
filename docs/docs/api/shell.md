---
title: Shell WebSocket API
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-drilling.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# Shell WebSocket API

Execute commands on remote agents through the mesh. The CLI handles this automatically - this reference is for building custom integrations.

**Using the CLI (recommended):**
```bash
muti-metroo shell abc123 whoami
muti-metroo shell --tty abc123 bash
```

**WebSocket endpoint for custom clients:**
```
ws://localhost:8080/agents/{agent-id}/shell?mode=tty
```

## WebSocket Endpoint

```
GET /agents/{agent-id}/shell?mode=tty|stream
```

### Query Parameters

| Parameter | Values | Description |
|-----------|--------|-------------|
| `mode` | `tty` | Interactive mode with PTY (default) |
| | `normal` | Normal mode without PTY |

### Subprotocol

The WebSocket uses the `muti-shell` subprotocol.

## Message Protocol

The WebSocket uses a binary message protocol. All messages have a 1-byte type prefix followed by the payload. The CLI handles this protocol automatically - you only need to understand it if building custom integrations.

### Message Types

| Name | Direction | Description |
|------|-----------|-------------|
| META | Client → Server | JSON metadata to start session |
| ACK | Server → Client | JSON acknowledgment |
| STDIN | Client → Server | Keyboard input (raw bytes) |
| STDOUT | Server → Client | Command output (raw bytes) |
| STDERR | Server → Client | Error output (raw bytes, normal mode only) |
| RESIZE | Client → Server | Terminal resize notification |
| SIGNAL | Client → Server | Signal to send (e.g., SIGINT) |
| EXIT | Server → Client | Process exit code |
| ERROR | Server → Client | JSON error message |

## Session Flow

### 1. Connect

```
WebSocket: ws://localhost:8080/agents/abc123/shell?mode=tty
Subprotocol: muti-shell
```

### 2. Send Metadata (META)

First message must be META with session configuration:

```json
{
  "command": "bash",
  "args": ["-l"],
  "env": {"TERM": "xterm-256color"},
  "work_dir": "/home/user",
  "password": "secret",
  "tty": {
    "rows": 24,
    "cols": 80,
    "term": "xterm-256color"
  },
  "timeout": 3600
}
```

| Field | Type | Description |
|-------|------|-------------|
| `command` | string | Command to execute (required) |
| `args` | string[] | Command arguments |
| `env` | object | Additional environment variables |
| `work_dir` | string | Working directory |
| `password` | string | Authentication password |
| `tty` | object | TTY settings (for interactive mode) |
| `tty.rows` | number | Terminal rows |
| `tty.cols` | number | Terminal columns |
| `tty.term` | string | TERM value (default: xterm-256color) |
| `timeout` | number | Session timeout in seconds |

### 3. Receive Acknowledgment (ACK)

```json
{
  "success": true,
  "error": ""
}
```

If `success` is false, the session failed to start.

### 4. Data Exchange

After ACK, send and receive data:

- **STDIN**: Send keyboard input as raw bytes
- **STDOUT**: Receive command output
- **STDERR**: Receive error output (normal mode only)
- **RESIZE**: Send terminal size changes
- **SIGNAL**: Send signals (e.g., SIGINT = 2)

### 5. Session End

The server sends EXIT with the exit code, then closes the WebSocket.

## Error Handling

### ERROR Message

```json
{
  "message": "command not allowed"
}
```

Sent when:
- Command not in whitelist
- Authentication failed
- Session limit reached
- Other server errors

### WebSocket Close Codes

| Code | Reason |
|------|--------|
| 1000 | Normal closure |
| 1002 | Protocol error |
| 1011 | Internal error |

## Custom Client Integration

For custom integrations, refer to the binary protocol implementation. The message format uses a 1-byte type prefix:

| Type Byte | Message |
|-----------|---------|
| 0x01 | META |
| 0x02 | ACK |
| 0x03 | STDIN |
| 0x04 | STDOUT |
| 0x05 | STDERR |
| 0x06 | RESIZE (4 bytes: rows, cols as uint16 BE) |
| 0x07 | SIGNAL (1 byte: signal number) |
| 0x08 | EXIT (4 bytes: exit code as int32 BE) |
| 0x09 | ERROR |

For a complete implementation example, see the CLI source code in the project repository.

## See Also

- [Shell Feature](/features/shell) - Feature overview
- [CLI - Shell](/cli/shell) - CLI reference
- [Shell Configuration](/configuration/shell) - Enable and configure shell access
