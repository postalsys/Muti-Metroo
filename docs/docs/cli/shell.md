---
title: shell
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-drilling.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# muti-metroo shell

Execute commands on remote agents through the mesh. Run one-off commands or open a full interactive terminal session.

**Quick examples:**
```bash
# Run a command
muti-metroo shell abc123 whoami

# Monitor system resources interactively
muti-metroo shell --tty abc123 htop

# Follow logs in real-time
muti-metroo shell abc123 journalctl -f
```

## Usage

```bash
muti-metroo shell [flags] <target-agent-id> [command] [args...]
```

## Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: Shell password for authentication
- `-t, --timeout <duration>`: Session timeout as duration string, e.g., `30s`, `5m` (default: 0 = no timeout)
- `--tty`: Interactive mode with PTY (for vim, htop, top, etc.)

:::tip
- **Default command**: If no command is specified, defaults to `bash`
- **Agent ID prefix**: You can use a short agent ID prefix (e.g., `abc123`) instead of the full 32-character ID. The prefix is automatically resolved.
:::

## Modes

### Normal Mode (Default)

Runs commands without PTY allocation. Commands execute and return an exit code:

```bash
# Simple commands
muti-metroo shell abc123 whoami
muti-metroo shell abc123 hostname
muti-metroo shell abc123 ls -la /tmp

# Follow logs
muti-metroo shell abc123 journalctl -u muti-metroo -f

# Tail a file
muti-metroo shell abc123 tail -f /var/log/syslog
```

### Interactive Mode (--tty)

Use `--tty` for programs that require a terminal (vim, htop, top):

```bash
# Run htop for resource monitoring
muti-metroo shell --tty abc123 htop

# Run top for process monitoring
muti-metroo shell --tty abc123 top

# Run vim to edit configuration
muti-metroo shell --tty abc123 vim /etc/muti-metroo/config.yaml
```

#### Windows Examples

```bash
# List running processes
muti-metroo shell abc123 tasklist

# Get system information
muti-metroo shell abc123 systeminfo

# View network connections
muti-metroo shell abc123 netstat -an
```

## Examples

```bash
# Simple command (normal mode)
muti-metroo shell abc123 whoami

# Follow logs (normal mode)
muti-metroo shell abc123 journalctl -u nginx -f

# Monitor resources interactively (requires --tty)
muti-metroo shell --tty abc123 htop

# With password authentication
muti-metroo shell -p secret abc123 whoami

# Via different agent
muti-metroo shell -a 192.168.1.10:8080 --tty abc123 top

# With session timeout (1 hour)
muti-metroo shell -t 3600 --tty abc123 htop
```

## Terminal Features

In interactive mode:

- Window resize is automatically forwarded (SIGWINCH)
- Ctrl+C sends SIGINT to remote process
- Full terminal emulation (colors, cursor movement)

## Exit Codes

The command exits with:
- The remote command's exit code on success
- 1 on connection or protocol errors

## See Also

- [Shell Feature](/features/shell) - Detailed shell documentation
- [Configuration](/configuration/overview) - Shell configuration
