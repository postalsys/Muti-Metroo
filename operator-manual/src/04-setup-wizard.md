# Setup Wizard

The interactive setup wizard is the easiest way to configure Muti Metroo. It guides you through all configuration steps with sensible defaults.

## Running the Wizard

```bash
muti-metroo setup
```

## Wizard Steps

### Step 1: Basic Setup

- **Data directory**: Where agent state is stored (default: `./data`)
- **Config file location**: Where to save the configuration (default: `./config.yaml`)

### Step 2: Agent Identity

- **Agent ID**: Auto-generate or specify a custom 128-bit hex string
- **Display name**: Human-readable name shown in the web dashboard

### Step 3: Agent Role

Choose one or more roles for your agent:

| Role | Description |
|------|-------------|
| **Ingress** | SOCKS5 proxy for client connections |
| **Transit** | Relay traffic between peers |
| **Exit** | Open connections to external destinations |
| **Combined** | Multiple roles on one agent |

### Step 4: Transport Configuration

- **Transport type**: QUIC (recommended), HTTP/2, or WebSocket
- **Listen address**: IP and port to listen on (e.g., `0.0.0.0:4433`)
- **TLS certificates**: Generate new, paste existing, or use file paths

### Step 5: Peer Connections

Optionally connect to existing mesh agents:

- Enter peer address
- Specify expected peer ID (for validation)
- Configure TLS settings (CA certificate)

### Step 6: SOCKS5 Configuration (Ingress Role)

If ingress role is selected:

- **Listen address**: Where to accept SOCKS5 connections
- **Authentication**: Enable/disable, add users with passwords
- **Connection limits**: Maximum concurrent connections

### Step 7: Exit Configuration (Exit Role)

If exit role is selected:

- **Routes**: CIDR blocks to advertise (e.g., `10.0.0.0/8`, `0.0.0.0/0`)
- **DNS servers**: Servers for domain resolution
- **DNS timeout**: Query timeout

### Step 8: Advanced Options

- **Logging**: Level (debug/info/warn/error) and format (text/json)
- **HTTP API**: Health checks and dashboard
- **Remote shell**: Remote command execution (disabled by default)
- **File transfer**: File upload/download (disabled by default)

### Step 9: Configuration Delivery

Choose how to deploy the configuration:

| Option | Description |
|--------|-------------|
| **Save to file** | Traditional `config.yaml` file (default) |
| **Embed in binary** | Single-file deployment with config baked in |

When embedding:
- **Service name**: Custom name for the service (default: `muti-metroo`)
- **Output path**: Where to save the embedded binary

### Step 10: Review and Save

The wizard displays the complete configuration for review before saving.

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
Display name []: Gateway Agent

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
CA Name [Mesh CA]:
Generated: ./certs/ca.crt, ./certs/ca.key

Generating Agent Certificate...
Common name [gateway-agent]:
DNS names (comma-separated) []:
Generated: ./certs/gateway-agent.crt, ./certs/gateway-agent.key

[5/9] Peer Connections
----------------------
Add peer connection? [n]: n

[6/9] SOCKS5 Configuration
--------------------------
SOCKS5 listen address [127.0.0.1:1080]:
Enable authentication? [n]:

[7/9] Exit Configuration
------------------------
Routes to advertise (CIDR, comma-separated):
  [0.0.0.0/0]:
DNS servers [8.8.8.8:53,1.1.1.1:53]:

[8/9] Advanced Options
----------------------
Log level [info]:
Enable HTTP API? [y]:
HTTP API address [:8080]:

[9/9] Review Configuration
--------------------------
(Configuration displayed here)

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

### Start the Agent Manually

If you did not start during the wizard:

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

## Re-running the Wizard

You can run the wizard again to modify configuration:

```bash
muti-metroo setup

# Existing config detected
Use existing configuration as base? [y]: y
```

The wizard will load existing values as defaults.

## Embedded Configuration

For single-file deployments, the wizard can embed configuration directly into the binary.

### Creating an Embedded Binary

During setup, choose "Embed in binary" when prompted:

```
Configuration Delivery
----------------------
> 1. Save to config file (traditional)
  2. Embed in binary (single-file deployment)

Delivery method [1]: 2
Service name [muti-metroo]: my-agent
Output binary path: ./my-agent
```

### Running Embedded Binary

```bash
# No config file needed
./my-agent run

# Output shows embedded config detection
Using embedded configuration
Starting Muti Metroo agent...
Display Name: my-agent
```

### Editing Embedded Configuration

To modify config in an existing embedded binary:

```bash
# Use regular binary to edit embedded one
muti-metroo setup -c /path/to/embedded-binary
```

The wizard extracts existing config as defaults, guides you through changes, and writes updated config back to the binary.

### Updating Deployed Agents

```bash
# Stop service
sudo systemctl stop my-agent

# Edit embedded config
muti-metroo setup -c /usr/local/bin/my-agent

# Restart service
sudo systemctl start my-agent
```

### Binary Format

```
[executable][XOR'd config][8-byte length][8-byte magic]
```

The XOR obfuscation prevents casual inspection but is not cryptographic security.

## Service Installation via Wizard

When running the wizard with elevated privileges, you can install as a system service:

### Linux (as root)

```bash
sudo muti-metroo setup
# At the end, choose to install as service
```

### Windows (as Administrator)

```powershell
# Run as Administrator
.\muti-metroo.exe setup
```

The wizard will offer to:
1. Create configuration in the appropriate system location
2. Register the system service
3. Enable automatic startup
4. Start the service
