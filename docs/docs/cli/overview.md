---
title: CLI Overview
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole reading CLI docs" style={{maxWidth: '180px'}} />
</div>

# CLI Reference

Complete command-line interface reference for Muti Metroo.

## Control Socket vs HTTP API

CLI commands use two different interfaces to communicate with agents:

### Control Socket (Local Only)

The **control socket** is a Unix socket for querying the local agent's state. It provides read-only access to status information.

| Aspect | Details |
|--------|---------|
| **Commands** | `status`, `peers`, `routes` |
| **Access** | Local machine only (Unix socket) |
| **Authentication** | None (filesystem permissions) |
| **Configuration** | `control.socket_path` (default: `./data/control.sock`) |
| **Use case** | Quick local status checks, scripting |

```bash
# Query local agent
muti-metroo status -s ./data/control.sock
muti-metroo peers -s ./data/control.sock
muti-metroo routes -s ./data/control.sock
```

### HTTP API (Local and Remote)

The **HTTP API** provides full management capabilities including remote agent operations across the mesh.

| Aspect | Details |
|--------|---------|
| **Commands** | `rpc`, `upload`, `download` |
| **Access** | Network accessible (TCP) |
| **Authentication** | Password required for RPC and file transfer |
| **Configuration** | `http.address` (default: `:8080`) |
| **Use case** | Remote operations, mesh-wide management |

```bash
# Execute command on remote agent (through local HTTP API)
muti-metroo rpc -a localhost:8080 <target-agent-id> whoami

# Upload file to remote agent
muti-metroo upload -a localhost:8080 <target-agent-id> ./file.txt /tmp/file.txt
```

### When to Use Each

| Task | Interface | Command |
|------|-----------|---------|
| Check if local agent is running | Control Socket | `muti-metroo status -s ./data/control.sock` |
| List local agent's peers | Control Socket | `muti-metroo peers -s ./data/control.sock` |
| View local route table | Control Socket | `muti-metroo routes -s ./data/control.sock` |
| Execute command on remote agent | HTTP API | `muti-metroo rpc <agent-id> <command>` |
| Transfer files to/from remote agent | HTTP API | `muti-metroo upload/download` |
| Access metrics, dashboard | HTTP API | Browser or curl to `http://localhost:8080` |

:::tip
The control socket is faster for local queries since it bypasses TCP. Use it for health checks and monitoring scripts. The HTTP API is required for any operation involving remote agents.
:::

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
| `status` | Show agent status via control socket |
| `peers` | List connected peers via control socket |
| `routes` | List route table via control socket |
| `rpc` | Execute remote procedure call |
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
muti-metroo cert ca -n "My CA"

# Generate password hash for config
muti-metroo hash --cost 12

# Check agent status via control socket
muti-metroo status -s ./data/control.sock

# List connected peers
muti-metroo peers -s ./data/control.sock

# List route table
muti-metroo routes -s ./data/control.sock

# Execute remote command
muti-metroo rpc agent123 whoami

# Upload file
muti-metroo upload agent123 local.txt /tmp/remote.txt
```
