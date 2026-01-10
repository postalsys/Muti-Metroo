---
title: CLI Overview
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole reading CLI docs" style={{maxWidth: '180px'}} />
</div>

# CLI Reference

Everything you need to set up agents, manage certificates, transfer files, and run commands on remote systems - all from the command line.

**Quick reference:**

| I want to... | Command |
|--------------|---------|
| Set up a new agent interactively | `muti-metroo setup` |
| Start an agent | `muti-metroo run -c config.yaml` |
| Create TLS certificates | `muti-metroo cert ca` / `muti-metroo cert agent` |
| Generate a password hash | `muti-metroo hash` |
| Run a command on a remote agent | `muti-metroo shell <agent-id> <command>` |
| Transfer files | `muti-metroo upload` / `muti-metroo download` |
| Test if a listener is reachable | `muti-metroo probe <address>` |
| Install as a system service | `muti-metroo service install` |

## HTTP API

All CLI commands that query agent state use the HTTP API to communicate with agents.

| Aspect | Details |
|--------|---------|
| **Local queries** | `status`, `peers`, `routes` |
| **Remote operations** | `shell`, `upload`, `download` |
| **Default address** | `localhost:8080` |
| **Configuration** | `http.address` in config |

```bash
# Query local agent (default: localhost:8080)
muti-metroo status
muti-metroo peers
muti-metroo routes

# Query different agent
muti-metroo status -a 192.168.1.10:8080
muti-metroo peers -a 192.168.1.10:8080

# Execute command on remote agent
muti-metroo shell <target-agent-id> whoami
muti-metroo shell --tty <target-agent-id> bash

# Transfer files
muti-metroo upload <target-agent-id> ./file.txt /tmp/file.txt
muti-metroo download <target-agent-id> /tmp/file.txt ./file.txt
```

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
| `hash` | Generate bcrypt password hash |
| `status` | Show agent status via HTTP API |
| `peers` | List connected peers via HTTP API |
| `routes` | List route table via HTTP API |
| `probe` | Test connectivity to a listener (standalone) |
| `shell` | Interactive or streaming remote shell |
| `upload` | Upload file to remote agent |
| `download` | Download file from remote agent |
| `service` | Service management (install, uninstall, status) |
| `management-key` | Generate and manage mesh topology encryption keys |

## Quick Examples

```bash
# Start agent
muti-metroo run -c config.yaml

# Interactive setup
muti-metroo setup

# Generate CA
muti-metroo cert ca --cn "My CA"

# Generate password hash for config
muti-metroo hash --cost 12

# Check agent status
muti-metroo status

# Check agent on different port
muti-metroo status -a localhost:9090

# Test connectivity to a listener (no running agent needed)
muti-metroo probe server.example.com:4433
muti-metroo probe --transport h2 server.example.com:443

# List connected peers
muti-metroo peers

# List route table
muti-metroo routes

# Execute remote command
muti-metroo shell agent123 whoami
muti-metroo shell --tty agent123 bash

# Upload file
muti-metroo upload agent123 local.txt /tmp/remote.txt
```
