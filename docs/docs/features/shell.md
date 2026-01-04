---
title: Remote Shell
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-digging.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# Remote Shell

Execute shell commands on remote agents with support for both interactive and streaming modes:

- **Interactive TTY**: Full terminal support for programs like vim, bash, top
- **Streaming mode**: Continuous output for commands like `journalctl -f` or `tail -f`
- **One-shot commands**: Quick command execution with streaming mode

## Configuration

```yaml
shell:
  enabled: false              # Disabled by default (security)
  whitelist: []               # Commands allowed (empty = none, ["*"] = all)
  # whitelist:
  #   - bash
  #   - vim
  #   - whoami
  #   - hostname
  password_hash: ""           # bcrypt hash of shell password
  timeout: 0s                 # Optional command timeout (0 = no timeout)
  max_sessions: 10            # Max concurrent sessions
```

:::tip Generate Password Hash
Use the built-in CLI to generate bcrypt hashes: `muti-metroo hash`

See [CLI - hash](/cli/hash) for details.
:::

## Security Model

1. **Command Whitelist**: Only whitelisted commands can run
   - Empty list = no commands allowed (default)
   - `["*"]` = all commands allowed (testing only!)
   - Commands must be base names only (no paths)

2. **Password Authentication**: bcrypt-hashed password required when configured

3. **Session Limits**: Maximum concurrent sessions to prevent resource exhaustion

4. **Argument Validation**: Dangerous shell metacharacters are blocked

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
- Ideal for one-shot command execution
- No terminal control characters

```bash
# Long-running streaming commands
muti-metroo shell --stream abc123 journalctl -u muti-metroo -f
muti-metroo shell --stream abc123 tail -f /var/log/syslog

# One-shot commands
muti-metroo shell --stream abc123 whoami
muti-metroo shell --stream abc123 ls -la /tmp
```

## CLI Usage

```bash
muti-metroo shell [flags] <agent-id> [command] [args...]

# Interactive bash (default)
muti-metroo shell abc123

# Interactive with specific command
muti-metroo shell abc123 bash
muti-metroo shell abc123 vim /etc/hosts

# Streaming mode (for one-shot or long-running commands)
muti-metroo shell --stream abc123 whoami
muti-metroo shell --stream abc123 journalctl -f

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

## Related

- [CLI - Shell](../cli/shell) - CLI reference
- [API - Shell](../api/shell) - WebSocket API reference
- [Security - Access Control](../security/access-control) - Whitelist configuration
