---
title: shell
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-digging.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# muti-metroo shell

Run commands on a remote agent.

## Usage

```bash
muti-metroo shell [flags] <target-agent-id> [command] [args...]
```

## Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: Shell password for authentication
- `-t, --timeout <seconds>`: Session timeout (default: 0 = no timeout)
- `--tty`: Interactive mode with PTY (for vim, bash, htop, etc.)

## Modes

### Streaming Mode (Default)

Runs commands without PTY allocation. Suitable for simple commands and continuous output:

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

Use `--tty` for programs that require a terminal (vim, bash, htop):

```bash
# Open bash shell
muti-metroo shell --tty abc123 bash

# Open default shell (bash)
muti-metroo shell --tty abc123

# Run vim
muti-metroo shell --tty abc123 vim /etc/config.yaml

# Run htop
muti-metroo shell --tty abc123 htop
```

## Examples

```bash
# Simple command (streaming mode)
muti-metroo shell abc123 whoami

# Follow logs (streaming mode)
muti-metroo shell abc123 journalctl -u nginx -f

# Interactive shell (requires --tty)
muti-metroo shell --tty abc123 bash

# With password authentication
muti-metroo shell -p secret abc123 whoami

# Via different agent
muti-metroo shell -a 192.168.1.10:8080 --tty abc123 top

# With session timeout (1 hour)
muti-metroo shell -t 3600 --tty abc123 bash
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

- [Shell Feature](../features/shell) - Detailed shell documentation
- [Configuration](../configuration/overview) - Shell configuration
