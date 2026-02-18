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

:::tip Configuration
See [Remote Shell Configuration](/configuration/shell) for all options including command whitelist, password authentication, and session limits.
:::

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

### Available Shells

Each agent automatically detects installed shells at startup. When `shell.enabled` is `true`, the agent advertises both the detected shells and a `shell_enabled` flag to the mesh via node info. You can see which shells are available on any agent through the [Dashboard API](/api/dashboard).

When shell is disabled, neither the shell list nor the `shell_enabled` flag appears in the topology -- making it clear that shell access is not available on that agent.

**Probed shells by platform:**

| Platform | Shells probed (in preference order) |
|----------|--------------------------------------|
| Linux/macOS | bash, sh, zsh, fish, ash, dash, ksh |
| Windows | powershell.exe, pwsh.exe, cmd.exe |

Shell detection is separate from the command whitelist:
- **Detection** reports which shells are installed on the system
- **Whitelist** controls which commands are allowed to execute

A shell appearing in the detected list does not mean it can be used -- it must also be in the agent's `whitelist` configuration.

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

## Common Workflows

### System Health Check

```bash
# Quick system status
muti-metroo shell abc123 uptime
muti-metroo shell abc123 df -h
muti-metroo shell abc123 free -m

# Check running services
muti-metroo shell abc123 systemctl status muti-metroo
```

### Log Monitoring

```bash
# Follow service logs
muti-metroo shell abc123 journalctl -u muti-metroo -f

# Search recent logs
muti-metroo shell abc123 journalctl -u muti-metroo --since "1 hour ago"
```

### Configuration Management

```bash
# View config (streaming mode)
muti-metroo shell abc123 cat /etc/muti-metroo/config.yaml

# Edit config (interactive mode)
muti-metroo shell --tty abc123 vim /etc/muti-metroo/config.yaml

# Restart service after changes
muti-metroo shell abc123 systemctl restart muti-metroo
```

## Troubleshooting

### Command Not Allowed

```
Error: command not in whitelist: vim
```

**Cause:** The command is not in the agent's `whitelist` configuration.

**Solutions:**
- Use only whitelisted commands
- Contact the agent administrator to add the command
- Check available commands in the agent's config

### Authentication Failed

```
Error: authentication required
```

**Cause:** The agent requires password authentication for shell access.

**Solution:** Provide the password with `-p`:
```bash
muti-metroo shell -p mypassword abc123 whoami
```

### Session Limit Reached

```
Error: maximum sessions exceeded
```

**Cause:** The agent has reached its `max_sessions` limit.

**Solutions:**
- Wait for existing sessions to complete
- Close unused shell sessions
- Contact the agent administrator to increase the limit

### Interactive Program Not Working

```
# vim appears broken, no colors in htop
```

**Cause:** Running an interactive program without `--tty` flag.

**Solution:** Use `--tty` for interactive programs:
```bash
muti-metroo shell --tty abc123 htop
muti-metroo shell --tty abc123 vim /etc/config.yaml
```

### Agent Not Reachable

```
Error: no route to agent abc123
```

**Cause:** The target agent is not connected to the mesh.

**Solutions:**
```bash
# Check if agent is known
curl http://localhost:8080/agents

# Verify connectivity
curl http://localhost:8080/healthz | jq '.peers'
```

## Related

- [CLI - Shell](/cli/shell) - CLI reference
- [API - Shell](/api/shell) - WebSocket API reference
- [Configuration - Shell](/configuration/shell) - Shell configuration reference
