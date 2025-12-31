---
title: Remote Procedure Call (RPC)
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting RPC" style={{maxWidth: '180px'}} />
</div>

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
  password_hash: "$2a$10$..."  # bcrypt hash (generate with: muti-metroo hash)
  timeout: 60s
```

:::tip Generate Password Hash
Use the built-in CLI to generate bcrypt hashes: `muti-metroo hash --cost 12`

See [CLI - hash](/cli/hash) for details.
:::

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

## Related

- [CLI - RPC](../cli/rpc) - CLI reference
- [API - RPC](../api/rpc) - HTTP API reference
- [Security - Access Control](../security/access-control) - Whitelist configuration
- [Troubleshooting - Common Issues](../troubleshooting/common-issues) - RPC issues
