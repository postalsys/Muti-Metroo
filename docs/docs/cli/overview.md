---
title: CLI Overview
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole reading CLI docs" style={{maxWidth: '180px'}} />
</div>

# CLI Reference

Complete command-line interface reference for Muti Metroo.

## Global Flags

Available for all commands:

- `-h, --help`: Show help for command
- `-v, --version`: Show version information

## Commands

| Command | Description |
|---------|-------------|
| `run` | Run agent with configuration file |
| `init` | Initialize agent identity |
| `setup` | Interactive setup wizard |
| `cert` | Certificate management (CA, agent, client) |
| `rpc` | Execute remote procedure call |
| `upload` | Upload file to remote agent |
| `download` | Download file from remote agent |
| `service` | Service management (install, uninstall, status) |

## Quick Examples

```bash
# Start agent
muti-metroo run -c config.yaml

# Interactive setup
muti-metroo setup

# Generate CA
muti-metroo cert ca -n "My CA"

# Execute remote command
muti-metroo rpc agent123 whoami

# Upload file
muti-metroo upload agent123 local.txt /tmp/remote.txt
```
