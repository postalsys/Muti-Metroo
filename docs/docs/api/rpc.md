---
title: RPC Endpoints
---

# RPC Endpoints

Remote procedure call execution.

## POST /agents/\{agent-id\}/rpc

Execute shell command on remote agent.

**Request:**
```json
{
  "password": "your-rpc-password",
  "command": "whoami",
  "args": ["-a"],
  "stdin": "base64-encoded-input",
  "timeout": 30
}
```

**Response:**
```json
{
  "exit_code": 0,
  "stdout": "base64-encoded-output",
  "stderr": "base64-encoded-errors",
  "error": ""
}
```

## Request Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `password` | string | Yes (if configured) | RPC password |
| `command` | string | Yes | Command to execute |
| `args` | []string | No | Command arguments |
| `stdin` | string | No | Base64-encoded stdin |
| `timeout` | int | No | Timeout in seconds (default 60) |

## Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `exit_code` | int | Command exit code |
| `stdout` | string | Base64-encoded stdout |
| `stderr` | string | Base64-encoded stderr |
| `error` | string | Error message if failed |

## Examples

```bash
# Execute whoami
curl -X POST http://localhost:8080/agents/abc123/rpc   -H "Content-Type: application/json"   -d '{
    "password": "secret",
    "command": "whoami"
  }'

# Execute ls with args
curl -X POST http://localhost:8080/agents/abc123/rpc   -H "Content-Type: application/json"   -d '{
    "password": "secret",
    "command": "ls",
    "args": ["-la", "/tmp"]
  }'
```

## Security

- Requires `rpc.enabled: true` in config
- Command must be in `rpc.whitelist`
- Password must match `rpc.password_hash`

See [RPC Feature](../features/rpc) for configuration.
