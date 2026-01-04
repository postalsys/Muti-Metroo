---
title: shell
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-digging.png" alt="Mole accessing shell" style={{maxWidth: '180px'}} />
</div>

# muti-metroo shell

Open an interactive shell or run streaming commands on a remote agent.

## Usage

```bash
muti-metroo shell [flags] <target-agent-id> [command] [args...]
```

## Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: Shell password for authentication
- `-t, --timeout <seconds>`: Session timeout (default: 0 = no timeout)
- `--stream`: Non-interactive streaming mode (no PTY)

## Modes

### Interactive Mode (Default)

Allocates a PTY on the remote agent for full terminal support:

```bash
# Open bash shell
muti-metroo shell abc123 bash

# Open default shell (bash)
muti-metroo shell abc123

# Run vim
muti-metroo shell abc123 vim /etc/config.yaml

# Run htop
muti-metroo shell abc123 htop
```

### Streaming Mode

Use `--stream` for commands with continuous output (no terminal features):

```bash
# Follow logs
muti-metroo shell --stream abc123 journalctl -u muti-metroo -f

# Tail a file
muti-metroo shell --stream abc123 tail -f /var/log/syslog

# Watch command
muti-metroo shell --stream abc123 watch -n 1 df -h
```

## Examples

```bash
# Basic interactive shell
muti-metroo shell abc123 bash

# With password authentication
muti-metroo shell -p secret abc123 bash

# Via different agent
muti-metroo shell -a 192.168.1.10:8080 abc123 top

# Streaming journalctl
muti-metroo shell --stream abc123 journalctl -u nginx -f

# With session timeout (1 hour)
muti-metroo shell -t 3600 abc123 bash
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
- [RPC Command](./rpc) - One-shot command execution
- [Configuration](../configuration/overview) - Shell configuration
