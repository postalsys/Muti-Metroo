---
title: Interactive Setup
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-idea.png" alt="Mole with bright idea" style={{maxWidth: '180px'}} />
</div>

# Interactive Setup Wizard

Get a working tunnel in under 5 minutes. The wizard asks you a few questions and generates everything you need - certificates, configuration, and agent identity.

By the end, you will have a SOCKS5 proxy ready to tunnel traffic through your mesh.

## Running the Wizard

```bash
muti-metroo setup
```

## What the Wizard Configures

The wizard walks you through 13 steps, though some are conditional based on your role selections.

### 1. Basic Setup

- **Config file path**: Where to save the configuration (default: `./config.yaml`)
- **Data directory**: Where agent state is stored (default: `./data`)
- **Display name**: Human-readable name for the web dashboard (optional)

### 2. Agent Role

Choose what this agent will do:

| Role | What It Does |
|------|--------------|
| **Ingress** | Accept your SOCKS5 connections (curl, SSH, browser) |
| **Transit** | Relay traffic to other agents in the mesh |
| **Exit** | Open connections to destinations (internet, internal networks) |

Most setups need at least one ingress (where you connect) and one exit (where traffic goes). A single agent can do all three.

### 3. Network Configuration

- **Transport type**: QUIC (recommended), HTTP/2, or WebSocket
- **Listen address**: IP and port to listen on
- **HTTP path**: Path for HTTP-based transports (default: `/mesh`)

### 4. TLS Configuration

- **Certificate setup**: Generate new, paste existing, or use file paths
- **Certificates directory**: Where to store/find certificates
- **mTLS**: Enable mutual TLS for peer authentication

### 5. Peer Connections

Optionally connect to existing mesh agents:

- Enter peer address (host:port)
- Specify expected agent ID (or `auto` for first connection)
- Select transport type
- Configure TLS verification
- Connectivity test with retry/skip options

### 6. SOCKS5 Configuration (Ingress Role)

If ingress role is selected:

- **Listen address**: Where to accept SOCKS5 connections
- **Authentication**: Enable/disable, add username and password

### 7. Exit Configuration (Exit Role)

If exit role is selected:

- **CIDR routes**: Networks to advertise (e.g., `0.0.0.0/0`, `::/0`)
- **Domain routes**: Domain patterns for domain-based routing (optional)

### 8. Advanced Options

- **Log level**: Debug, Info (default), Warning, or Error
- **HTTP API**: Enable health check endpoint for monitoring and CLI

### 9. Remote Shell Access

- **Enable shell**: Allow remote command execution (disabled by default)
- **Password**: Required for shell authentication (min 8 characters)
- **Command whitelist**: No commands, all commands, or custom list

### 10. File Transfer

- **Enable file transfer**: Allow file upload/download (disabled by default)
- **Password**: Required for file transfer authentication
- **Max file size**: Limit in MB (default: 500 MB)
- **Path restrictions**: Allow all paths or restrict to specific directories

### 11. Management Key Encryption

OPSEC protection for red team operations:

- **Skip**: No encryption (not recommended for red team ops)
- **Generate new**: Create a new management keypair
- **Enter existing**: Use an existing public key

When enabled, mesh topology data is encrypted so compromised agents cannot reveal the network structure.

### 12. Configuration Delivery

Choose how to deploy the configuration:

| Option | Description |
|--------|-------------|
| **Save to file** | Traditional `config.yaml` file (default) |
| **Embed in binary** | Single-file deployment with config baked in |

For both options, you specify a **service name** used when installing as a system service (default: `muti-metroo`).

When embedding:
- **Output path**: Where to save the embedded binary

### 13. Service Installation (Optional)

If running with elevated privileges, optionally install as a system service:

- **Linux**: systemd service (requires root)
- **macOS**: launchd service (requires root)
- **Windows**: Windows Service (requires Administrator)

## Example Wizard Session

```
$ muti-metroo setup

================================================================================
                        Muti Metroo Setup Wizard
================================================================================
                    Userspace Mesh Networking Agent

--------------------------------------------------------------------------------
Basic Setup
--------------------------------------------------------------------------------
Configure the essential paths for your agent.

Config File Path [./config.yaml]:
Data Directory [./data]:
Display Name (press Enter to use Agent ID) []: My Gateway Agent

--------------------------------------------------------------------------------
Agent Role
--------------------------------------------------------------------------------
Select the roles this agent will perform.
You can select multiple roles.

  1. [ ] Ingress (SOCKS5 proxy entry point)
  2. [ ] Transit (relay traffic between peers)
  3. [ ] Exit (connect to external networks)

Select Roles (comma-separated) []: 1,3

--------------------------------------------------------------------------------
Network Configuration
--------------------------------------------------------------------------------
Configure how this agent listens for connections.

Transport Protocol (QUIC is recommended for best performance):

> 1. QUIC (UDP, fastest)
  2. HTTP/2 (TCP, firewall-friendly)
  3. WebSocket (TCP, proxy-friendly)

Select [1]:
Listen Address [0.0.0.0:4433]:

--------------------------------------------------------------------------------
TLS Configuration
--------------------------------------------------------------------------------
TLS is required for secure communication.
You can generate new certificates or use existing ones.

> 1. Generate new self-signed certificates (Recommended for testing)
  2. Paste certificate and key content
  3. Use existing certificate files

Certificate Setup [1]:
Certificates Directory [./data/certs]:
Enable mTLS (mutual TLS)? [Y/n]: y

--------------------------------------------------------------------------------
Generate Certificates
--------------------------------------------------------------------------------
A CA and server certificate will be generated.

Common Name [muti-metroo]:
Validity (days) [365]:

[OK] Generated CA certificate: ./data/certs/ca.crt
[OK] Generated server certificate: ./data/certs/server.crt
  Fingerprint: SHA256:abc123...

--------------------------------------------------------------------------------
Peer Connections
--------------------------------------------------------------------------------
Configure connections to other mesh agents.

Add peer connections? [y/N]: y

--------------------------------------------------------------------------------
Peer #1
--------------------------------------------------------------------------------

Peer Address (host:port) []: 192.168.1.50:4433
Expected Agent ID (hex string, or 'auto') [auto]:

> 1. QUIC
  2. HTTP/2
  3. WebSocket

Transport [1]:
Skip TLS verification? (only for testing with self-signed certs) [y/N]: y

[INFO] Testing connectivity to peer...
[INFO] Remote agent: Transit Node (abc123def456)
[INFO] Round-trip time: 5ms
[OK] Connected successfully!

Add another peer? [y/N]: n

--------------------------------------------------------------------------------
SOCKS5 Proxy
--------------------------------------------------------------------------------
Configure the SOCKS5 ingress proxy.

Listen Address [127.0.0.1:1080]:
Enable authentication? [y/N]: n

--------------------------------------------------------------------------------
Exit Node Configuration
--------------------------------------------------------------------------------
Configure this agent as an exit node.
It will allow traffic to specified networks.

Allowed Routes (CIDR) - one CIDR per line (e.g., 0.0.0.0/0 for all traffic):
Enter routes, one per line. Enter empty line to finish.
Route (or empty to finish) []: 10.0.0.0/8
Route (or empty to finish) []: 192.168.0.0/16
Route (or empty to finish) []:

--------------------------------------------------------------------------------
Domain Routes (Optional)
--------------------------------------------------------------------------------
Route traffic by domain name.
Examples: api.internal.corp, *.example.com

Enter domain patterns, one per line. Enter empty line to finish.
Domain (or empty to finish) []:

--------------------------------------------------------------------------------
Advanced Options
--------------------------------------------------------------------------------
Configure monitoring and logging.

  1. Debug (verbose)
> 2. Info (recommended)
  3. Warning
  4. Error (quiet)

Log Level [2]:
Enable health check endpoint? (HTTP endpoint for monitoring and CLI) [Y/n]: y

--------------------------------------------------------------------------------
Remote Shell Access
--------------------------------------------------------------------------------
Shell allows executing commands remotely on this agent.
Commands must be whitelisted for security.

Enable Remote Shell? (requires authentication) [y/N]: n

--------------------------------------------------------------------------------
File Transfer
--------------------------------------------------------------------------------
File transfer allows uploading and downloading files to/from this agent.
Files are transferred via the control channel.

Enable file transfer? (requires authentication) [y/N]: n

--------------------------------------------------------------------------------
Management Key Encryption (OPSEC Protection)
--------------------------------------------------------------------------------
Encrypt mesh topology data so only operators can view it.
Compromised agents will only see encrypted blobs.

This is recommended for red team operations.

> 1. Skip (not recommended for red team ops)
  2. Generate new management keypair
  3. Enter existing public key

Management Key Setup [1]:

--------------------------------------------------------------------------------
Configuration Delivery
--------------------------------------------------------------------------------
Choose how to deploy the configuration.
Embedding creates a single-file binary with config baked in.

> 1. Save to config file (traditional)
  2. Embed in binary (single-file deployment)

Delivery method [1]:
[INFO] Service name is used when installing as a system service.
Service name [muti-metroo]:

--------------------------------------------------------------------------------
[OK] Setup Complete!
--------------------------------------------------------------------------------

  Display Name:   My Gateway Agent
  Agent ID:       abc123def456789012345678901234567
  E2E Public Key: 0123456789abcdef0123456789abcdef...
  Config file:    ./config.yaml
  Data dir:       ./data

  Listener:     quic://0.0.0.0:4433
  SOCKS5:       127.0.0.1:1080
  Exit routes:  [10.0.0.0/8 192.168.0.0/16]
  HTTP API:     http://:8080

  To start the agent:
    muti-metroo run -c ./config.yaml
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

## Editing Embedded Configuration

To edit the configuration embedded in an existing binary:

```bash
# Edit embedded config in a deployed binary
muti-metroo setup -c /usr/local/bin/my-agent
```

The wizard will:
1. Detect the embedded configuration
2. Load existing values as defaults
3. Guide you through changes
4. Write updated configuration back to the binary

This is useful for updating deployed agents without redeploying:

```bash
# Stop the service
sudo systemctl stop my-agent

# Edit the embedded config
muti-metroo setup -c /usr/local/bin/my-agent

# Restart the service
sudo systemctl start my-agent
```

See [Embedded Configuration](/deployment/embedded-config) for more details.

## Next Steps

- [Your First Mesh](/getting-started/first-mesh) - Connect multiple agents
- [Configuration Reference](/configuration/overview) - All options explained
- [Deployment](/deployment/scenarios) - Production deployment patterns
