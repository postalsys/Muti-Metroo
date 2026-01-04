---
title: Shell WebSocket API
---

# Shell WebSocket API

The shell API uses WebSocket for bidirectional streaming communication.

## WebSocket Endpoint

```
GET /agents/{agent-id}/shell?mode=tty|stream
```

### Query Parameters

| Parameter | Values | Description |
|-----------|--------|-------------|
| `mode` | `tty` | Interactive mode with PTY (default) |
| | `stream` | Streaming mode without PTY |

### Subprotocol

The WebSocket uses the `muti-shell` subprotocol.

## Message Protocol

All messages are binary WebSocket frames with a 1-byte type prefix followed by the payload.

### Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| 0x01 | META | Client -> Server | JSON metadata |
| 0x02 | ACK | Server -> Client | JSON acknowledgment |
| 0x03 | STDIN | Client -> Server | Raw bytes |
| 0x04 | STDOUT | Server -> Client | Raw bytes |
| 0x05 | STDERR | Server -> Client | Raw bytes |
| 0x06 | RESIZE | Client -> Server | 4 bytes: rows, cols (uint16 BE) |
| 0x07 | SIGNAL | Client -> Server | 1 byte: signal number |
| 0x08 | EXIT | Server -> Client | 4 bytes: exit code (int32 BE) |
| 0x09 | ERROR | Server -> Client | JSON error |

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
- **STDERR**: Receive error output (streaming mode only)
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

## Example: JavaScript Client

```javascript
const ws = new WebSocket('ws://localhost:8080/agents/abc123/shell?mode=tty', ['muti-shell']);
ws.binaryType = 'arraybuffer';

// Send metadata
ws.onopen = () => {
  const meta = {
    command: 'bash',
    password: 'secret',
    tty: { rows: 24, cols: 80 }
  };
  const payload = new TextEncoder().encode(JSON.stringify(meta));
  const msg = new Uint8Array(1 + payload.length);
  msg[0] = 0x01; // META
  msg.set(payload, 1);
  ws.send(msg);
};

// Handle messages
ws.onmessage = (event) => {
  const data = new Uint8Array(event.data);
  const type = data[0];
  const payload = data.slice(1);

  switch (type) {
    case 0x02: // ACK
      console.log('Session started');
      break;
    case 0x04: // STDOUT
      terminal.write(payload);
      break;
    case 0x08: // EXIT
      const exitCode = new DataView(payload.buffer).getInt32(0, false);
      console.log('Exit code:', exitCode);
      break;
    case 0x09: // ERROR
      const error = JSON.parse(new TextDecoder().decode(payload));
      console.error('Error:', error.message);
      break;
  }
};

// Send stdin
function sendInput(text) {
  const payload = new TextEncoder().encode(text);
  const msg = new Uint8Array(1 + payload.length);
  msg[0] = 0x03; // STDIN
  msg.set(payload, 1);
  ws.send(msg);
}

// Send resize
function sendResize(rows, cols) {
  const msg = new Uint8Array(5);
  msg[0] = 0x06; // RESIZE
  new DataView(msg.buffer).setUint16(1, rows, false);
  new DataView(msg.buffer).setUint16(3, cols, false);
  ws.send(msg);
}
```

## See Also

- [Shell Feature](../features/shell) - Feature overview
- [CLI - Shell](../cli/shell) - CLI reference
