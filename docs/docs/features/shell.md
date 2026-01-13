---
title: Remote Shell
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-drilling.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# Remote Shell

Run commands on any agent in your mesh. Check system status, monitor resources, or edit configuration files - all through your encrypted tunnel.

```bash
# Run a quick command
muti-metroo shell abc123 whoami

# Monitor system resources interactively
muti-metroo shell --tty abc123 htop

# Edit a configuration file with vim
muti-metroo shell --tty abc123 vim /etc/muti-metroo/config.yaml
```

Two modes are available:
- **Normal mode**: Run commands and see output (default)
- **Interactive TTY**: Full terminal for vim, htop, top, and other interactive programs

## Configuration

```yaml
shell:
  enabled: false              # Disabled by default (security)
  whitelist: []               # Commands allowed (empty = none, ["*"] = all)
  # whitelist:
  #   - htop
  #   - top
  #   - vim
  #   - journalctl
  #   - systemctl
  password_hash: ""           # bcrypt hash of shell password
  timeout: 0s                 # Optional command timeout (0 = no timeout)
  max_sessions: 0             # Max concurrent sessions (0 = unlimited)
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

### Normal Mode (Default)

Standard execution without PTY allocation:

- Separate stdout and stderr streams
- Commands run until exit and return an exit code
- No terminal control characters

```bash
# Simple commands
muti-metroo shell abc123 whoami
muti-metroo shell abc123 ls -la /tmp

# Long-running streaming commands
muti-metroo shell abc123 journalctl -u muti-metroo -f
muti-metroo shell abc123 tail -f /var/log/syslog
```

### Interactive Mode (--tty)

Allocates a PTY (pseudo-terminal) on the remote agent:

- Full terminal emulation
- Supports terminal resize (SIGWINCH)
- Works with interactive programs (vim, less, htop)
- Single combined stdout/stderr stream

```bash
muti-metroo shell --tty abc123 htop
muti-metroo shell --tty abc123 vim /etc/muti-metroo/config.yaml
muti-metroo shell --tty abc123 top
```

## CLI Usage

```bash
muti-metroo shell [flags] <agent-id> [command] [args...]

# Simple command (normal mode, default)
muti-metroo shell abc123 whoami

# Follow logs (normal mode)
muti-metroo shell abc123 journalctl -f

# Monitor resources interactively (requires --tty)
muti-metroo shell --tty abc123 htop

# Interactive vim (requires --tty)
muti-metroo shell --tty abc123 vim /etc/muti-metroo/config.yaml

# With password
muti-metroo shell -p secret abc123 whoami

# Via different agent
muti-metroo shell -a 192.168.1.10:8080 --tty abc123 top
```

### Flags

- `-a, --agent`: Agent HTTP API address (default: localhost:8080)
- `-p, --password`: Shell password for authentication
- `-t, --timeout`: Session timeout in seconds (default: 0 = no timeout)
- `--tty`: Interactive mode with PTY (for vim, htop, top, etc.)

## WebSocket API

Shell sessions use WebSocket for bidirectional communication.

**Endpoint**: `GET /agents/{agent-id}/shell?mode=tty|stream`

See [API - Shell](/api/shell) for protocol details.

## Platform Support

| Platform | Interactive (PTY) | Normal |
|----------|-------------------|-----------|
| Linux    | Yes               | Yes       |
| macOS    | Yes               | Yes       |
| Windows  | Yes (ConPTY)      | Yes       |

:::info Windows PTY
Windows agents use ConPTY (Windows Pseudo Console) for interactive sessions. ConPTY is available on Windows 10 version 1809 and later.
:::

### Windows Examples

Run system management commands on Windows agents:

```bash
# List running processes
muti-metroo shell abc123 tasklist

# Get system information
muti-metroo shell abc123 systeminfo

# View network connections
muti-metroo shell abc123 netstat -an
```

## Related

- [CLI - Shell](/cli/shell) - CLI reference
- [API - Shell](/api/shell) - WebSocket API reference
- [Security - Access Control](/security/access-control) - Whitelist configuration
