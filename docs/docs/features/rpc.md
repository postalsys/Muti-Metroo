---
title: Remote Procedure Call (RPC)
---

# Remote Procedure Call (RPC)

Execute shell commands on remote agents for maintenance and diagnostics.

## Configuration

```yaml
rpc:
  enabled: true
  whitelist:
    - whoami
    - hostname
    - ip
  password_hash: "$2a$10$..."  # bcrypt hash
  timeout: 60s
```

## Security Model

Two-layer security:

1. **Command Whitelist**: Only whitelisted commands can run
   - Empty list = no commands allowed (default)
   - `["*"]` = all commands allowed (testing only!)
   - Specific commands: `["whoami", "hostname"]`

2. **Password Authentication**: bcrypt-hashed password required

## CLI Usage

```bash
muti-metroo rpc <agent-id> <command> [args...]

# Examples
muti-metroo rpc abc123 whoami
muti-metroo rpc abc123 ip addr show
muti-metroo rpc abc123 ls -la /tmp

# With password
muti-metroo rpc -p secret abc123 hostname

# Pipe stdin
echo "test" | muti-metroo rpc abc123 cat
```

### Flags

- `-a, --agent`: Agent HTTP API address (default: localhost:8080)
- `-p, --password`: RPC password
- `-t, --timeout`: Command timeout in seconds (default: 60)

## HTTP API

`POST /agents/{agent-id}/rpc`

Request:
```json
{
  "password": "secret",
  "command": "whoami",
  "args": ["-a"],
  "stdin": "base64-encoded-input",
  "timeout": 30
}
```

Response:
```json
{
  "exit_code": 0,
  "stdout": "base64-encoded-output",
  "stderr": "base64-encoded-errors",
  "error": ""
}
```

## Implementation Limits

- **Max stdin**: 1 MB
- **Max output**: 4 MB (stdout + stderr each)
- **Chunking**: Large payloads split into 14KB chunks with gzip
- **Timeout**: Default 60s, configurable per-request

## Metrics

- `muti_metroo_rpc_calls_total`: RPC calls by result
- `muti_metroo_rpc_call_duration_seconds`: Call duration
- `muti_metroo_rpc_bytes_received_total`: Request bytes
- `muti_metroo_rpc_bytes_sent_total`: Response bytes

Result labels: `success`, `failed`, `rejected`, `auth_failed`, `error`
