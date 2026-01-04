---
title: Interactive Shell
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-digging.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# Interactive Shell

Open interactive shell sessions or run streaming commands on remote agents. Unlike RPC which executes one-shot commands, shell supports:

- **Interactive TTY**: Full terminal support for programs like vim, bash, top
- **Streaming mode**: Continuous output for commands like `journalctl -f` or `tail -f`

## Configuration

```yaml
shell:
  enabled: false              # Disabled by default (security)
  streaming:
    enabled: true             # Allow streaming mode (--stream)
    max_duration: 24h         # Max session duration
  interactive:
    enabled: true             # Allow interactive TTY mode
    allowed_commands:         # Override whitelist for interactive mode
      - bash
      - sh
      - zsh
  whitelist:                  # Commands allowed (empty = use RPC whitelist)
    - bash
    - vim
    - top
  password_hash: ""           # bcrypt hash (empty = use RPC password)
  max_sessions: 10            # Max concurrent sessions
```

:::tip Generate Password Hash
Use the built-in CLI to generate bcrypt hashes: `muti-metroo hash --cost 12`

See [CLI - hash](/cli/hash) for details.
:::

## Security Model

Shell inherits security from the RPC feature with additional controls:

1. **Command Whitelist**: Only whitelisted commands can run
   - Empty list = use RPC whitelist
   - Interactive mode can have a separate `allowed_commands` list
   - `["*"]` = all commands allowed (testing only!)

2. **Password Authentication**: bcrypt-hashed password (falls back to RPC password)

3. **Session Limits**: Maximum concurrent sessions to prevent resource exhaustion

4. **Duration Limits**: Maximum session duration for streaming mode

## Modes

### Interactive Mode (Default)

Allocates a PTY (pseudo-terminal) on the remote agent:

- Full terminal emulation
- Supports terminal resize (SIGWINCH)
- Works with interactive programs (vim, less, htop)
- Single combined stdout/stderr stream

```bash
muti-metroo shell abc123 bash
muti-metroo shell abc123 vim /etc/config.yaml
muti-metroo shell abc123 htop
```

### Streaming Mode (--stream)

Non-interactive mode without PTY allocation:

- Separate stdout and stderr streams
- Suitable for long-running commands with continuous output
- No terminal control characters

```bash
muti-metroo shell --stream abc123 journalctl -u muti-metroo -f
muti-metroo shell --stream abc123 tail -f /var/log/syslog
muti-metroo shell --stream abc123 watch -n 1 df -h
```

## CLI Usage

```bash
muti-metroo shell [flags] <agent-id> [command] [args...]

# Interactive bash (default)
muti-metroo shell abc123

# Interactive with specific command
muti-metroo shell abc123 bash
muti-metroo shell abc123 vim /etc/hosts

# Streaming mode
muti-metroo shell --stream abc123 journalctl -f
muti-metroo shell --stream abc123 tail -f /var/log/app.log

# With password
muti-metroo shell -p secret abc123 bash

# Via different agent
muti-metroo shell -a 192.168.1.10:8080 abc123 top
```

### Flags

- `-a, --agent`: Agent HTTP API address (default: localhost:8080)
- `-p, --password`: Shell password for authentication
- `-t, --timeout`: Session timeout in seconds (default: 0 = no timeout)
- `--stream`: Non-interactive streaming mode (no PTY)

## WebSocket API

Shell sessions use WebSocket for bidirectional communication.

**Endpoint**: `GET /agents/{agent-id}/shell?mode=tty|stream`

See [API - Shell](../api/shell) for protocol details.

## Platform Support

| Platform | Interactive (PTY) | Streaming |
|----------|-------------------|-----------|
| Linux    | Yes               | Yes       |
| macOS    | Yes               | Yes       |
| Windows  | No                | Yes       |

:::note Windows Limitation
PTY allocation is not available on Windows. Use streaming mode (`--stream`) for Windows agents.
:::

## Metrics

- `muti_metroo_shell_sessions_active`: Active sessions gauge
- `muti_metroo_shell_sessions_total`: Total sessions by type and result
- `muti_metroo_shell_duration_seconds`: Session duration histogram
- `muti_metroo_shell_bytes_total`: Bytes transferred by direction

Type labels: `stream`, `interactive`
Result labels: `success`, `error`, `timeout`, `rejected`
Direction labels: `stdin`, `stdout`, `stderr`

## Comparison with RPC

| Feature | RPC | Shell |
|---------|-----|-------|
| Execution model | One-shot | Streaming |
| TTY support | No | Yes (interactive mode) |
| stdin | Single payload | Continuous |
| stdout/stderr | Buffered | Real-time streaming |
| Use case | Quick commands | Long-running/interactive |
| Protocol | HTTP POST | WebSocket |

## Related

- [CLI - Shell](../cli/shell) - CLI reference
- [API - Shell](../api/shell) - WebSocket API reference
- [RPC Feature](./rpc) - One-shot command execution
- [Security - Access Control](../security/access-control) - Whitelist configuration
