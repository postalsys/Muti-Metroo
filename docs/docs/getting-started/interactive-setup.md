---
title: Interactive Setup
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-idea.png" alt="Mole with bright idea" style={{maxWidth: '180px'}} />
</div>

# Interactive Setup Wizard

The easiest way to get started with Muti Metroo is using the interactive setup wizard. It guides you through all configuration steps with sensible defaults.

## Running the Wizard

```bash
muti-metroo setup
```

## What the Wizard Configures

The wizard walks you through the following steps:

### 1. Basic Setup

- **Data directory**: Where agent state is stored (default: `./data`)
- **Config file location**: Where to save the configuration (default: `./config.yaml`)

### 2. Agent Identity

- **Agent ID**: Auto-generate or specify a custom ID
- **Display name**: Human-readable name for the web dashboard

### 3. Agent Role

Choose the primary role for your agent:

| Role | Description |
|------|-------------|
| **Ingress** | SOCKS5 proxy for client connections |
| **Transit** | Relay traffic between peers |
| **Exit** | Open connections to external destinations |
| **Combined** | Multiple roles on one agent |

### 4. Transport Configuration

- **Transport type**: QUIC (recommended), HTTP/2, or WebSocket
- **Listen address**: IP and port to listen on
- **TLS certificates**: Generate new, paste existing, or use file paths

### 5. Peer Connections

Optionally connect to existing mesh agents:

- Enter peer address
- Specify expected peer ID (for validation)
- Configure TLS settings

### 6. SOCKS5 Configuration (Ingress Role)

If ingress role is selected:

- **Listen address**: Where to accept SOCKS5 connections
- **Authentication**: Enable/disable, add users with passwords
- **Connection limits**: Maximum concurrent connections

### 7. Exit Configuration (Exit Role)

If exit role is selected:

- **Routes**: CIDR blocks to advertise (e.g., `10.0.0.0/8`, `0.0.0.0/0`)
- **DNS servers**: Servers for domain resolution
- **DNS timeout**: Query timeout

### 8. Advanced Options

- **Logging**: Level (debug/info/warn/error) and format (text/json)
- **HTTP API**: Health checks, metrics, dashboard
- **Control socket**: Unix socket for CLI commands
- **RPC**: Remote command execution (disabled by default)
- **File transfer**: File upload/download (disabled by default)

### 9. Service Installation (Optional)

When running as root (Linux) or Administrator (Windows):

- Install as system service
- Configure automatic startup
- Set up service management

## Example Wizard Session

```
$ muti-metroo setup

Welcome to Muti Metroo Setup Wizard
===================================

[1/9] Basic Setup
-----------------
Data directory [./data]:
Config file [./config.yaml]:

[2/9] Agent Identity
--------------------
Agent ID [auto]:
Display name []: My Gateway Agent

[3/9] Agent Role
----------------
Select role(s):
  [x] Ingress (SOCKS5 proxy)
  [ ] Transit (relay only)
  [x] Exit (external connections)

[4/9] Transport Configuration
-----------------------------
Transport type [quic]:
Listen address [0.0.0.0:4433]:

TLS Certificate Setup:
  1. Generate new certificates
  2. Paste PEM content
  3. Use existing files

Choice [1]: 1

Generating Certificate Authority...
CA Name [My Mesh CA]:
Generated: ./certs/ca.crt, ./certs/ca.key

Generating Agent Certificate...
Common name [my-gateway-agent]:
DNS names (comma-separated) []: gateway.example.com
IP addresses (comma-separated) []:
Generated: ./certs/agent.crt, ./certs/agent.key

[5/9] Peer Connections
----------------------
Add peer connection? [n]: y
Peer address: 192.168.1.50:4433
Peer ID: abc123def456...
Peer CA certificate [./certs/ca.crt]:

Add another peer? [n]: n

[6/9] SOCKS5 Configuration
--------------------------
SOCKS5 listen address [127.0.0.1:1080]:
Enable authentication? [n]:
Max connections [1000]:

[7/9] Exit Configuration
------------------------
Routes to advertise (CIDR, comma-separated):
  [0.0.0.0/0]: 10.0.0.0/8,192.168.0.0/16
DNS servers [8.8.8.8:53,1.1.1.1:53]:

[8/9] Advanced Options
----------------------
Log level [info]:
Enable HTTP API? [y]:
HTTP API address [:8080]:
Enable web dashboard? [y]:

[9/9] Review Configuration
--------------------------

agent:
  id: "auto"
  display_name: "My Gateway Agent"
  data_dir: "./data"
  log_level: "info"

tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    # Uses global TLS settings

peers:
  - id: "abc123def456..."
    transport: quic
    address: "192.168.1.50:4433"
    # Uses global CA and cert/key

socks5:
  enabled: true
  address: "127.0.0.1:1080"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"

http:
  enabled: true
  address: ":8080"

Save configuration? [y]: y
Configuration saved to ./config.yaml

Start agent now? [y]: y
Starting agent...
```

## After the Wizard

Once the wizard completes:

1. **Configuration saved**: Your `config.yaml` is ready
2. **Certificates generated**: TLS certificates are in `./certs/`
3. **Agent initialized**: Identity stored in data directory

### Start the Agent

If you did not start it during the wizard:

```bash
muti-metroo run -c ./config.yaml
```

### Verify Operation

```bash
# Check health
curl http://localhost:8080/health

# View dashboard
open http://localhost:8080/ui/

# Test SOCKS5 (if exit is enabled)
curl -x socks5://localhost:1080 https://example.com
```

## Service Installation

When running the wizard with elevated privileges, you can install as a system service:

### Linux (as root)

```bash
sudo muti-metroo setup
# At the end, choose to install as service
```

The wizard will:
1. Create `/etc/muti-metroo/config.yaml`
2. Create systemd unit file
3. Enable and start the service

### Windows (as Administrator)

```powershell
# Run as Administrator
.\muti-metroo.exe setup
```

The wizard will:
1. Create configuration in `C:\ProgramData\muti-metroo\`
2. Register Windows Service
3. Start the service

## Re-running the Wizard

You can run the wizard again to modify configuration:

```bash
muti-metroo setup

# Existing config detected
Use existing configuration as base? [y]: y
```

The wizard will load existing values as defaults.

## Next Steps

- [Your First Mesh](first-mesh) - Connect multiple agents
- [Configuration Reference](../configuration/overview) - All options explained
- [Deployment](../deployment/scenarios) - Production deployment patterns
