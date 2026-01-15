---
title: Embedded Configuration
sidebar_position: 2
---

# Embedded Configuration

Drop a single executable. No config files, no setup - just run the binary and the agent starts with all settings baked in.

**What you get:**
- One file to transfer and deploy
- Nothing to configure on the target system
- Custom binary name for each deployment
- Can still edit config later without rebuilding

## How It Works

The configuration is appended to the binary itself:
- YAML config is XOR-obfuscated and added to the end of the executable
- The agent detects and loads embedded config automatically on startup
- No `-c` flag needed - just `./my-agent run`
- External config files are ignored when embedded config is present

## True Single-File Deployment

With embedded identity, the agent requires **zero external files**:

- **Agent ID** - Stored in config as `agent.id`
- **X25519 keypair** - Stored in config as `agent.private_key`
- **No data directory** - Not needed when identity is in config

The setup wizard offers this option automatically when embedding config.

## Creating an Embedded Binary

### Using the Setup Wizard

The easiest way to create an embedded binary:

```bash
muti-metroo setup
```

When prompted for "Configuration Delivery" (step 12 of 13), choose **"Embed in binary"**:

```
--- Configuration Delivery ----------------------------------------------------
Choose how to deploy the configuration.
Embedding creates a single-file binary with config baked in.

Delivery method:
  Save to config file (traditional)
> Embed in binary (single-file deployment)
```

You'll then specify:
- **Service name**: Custom name for the service (e.g., `my-agent`)
- **Output path**: Where to save the embedded binary

### Automatic Identity Embedding

When you choose to embed config, the wizard **automatically** embeds the identity:
- `agent.id` is set to the generated agent ID
- `agent.private_key` is set to the generated X25519 private key
- `agent.data_dir` is cleared (not needed)

The resulting binary can run with no external files - true single-file deployment.

### Binary Output

The wizard creates a new binary with the embedded config:

```bash
# Original binary size
$ ls -la muti-metroo
-rwxr-xr-x  1 user  staff  20753408 muti-metroo

# Embedded binary (config adds ~1-2KB)
$ ls -la my-agent
-rwxr-xr-x  1 user  staff  20755200 my-agent
```

## Running an Embedded Binary

Simply run the binary - no `run` command needed:

```bash
./my-agent
```

The wizard automatically sets `default_action: run` in embedded configs, so the agent starts without any arguments.

Output shows it's using embedded config:

```
Using embedded configuration
Starting Muti Metroo agent...
Display Name: my-agent
Agent ID: abc123def456...
```

Other commands still work normally:

```bash
./my-agent help     # Show help
./my-agent status   # Check status
./my-agent --version
```

## Editing Embedded Configuration

To modify the configuration in an existing embedded binary:

```bash
# Use a regular muti-metroo binary to edit the embedded one
muti-metroo setup -c /path/to/embedded-binary
```

The wizard will:
1. Detect and extract the embedded configuration
2. Use existing values as defaults
3. Guide you through changes
4. Write the updated configuration back to the binary

### Example: Updating a Deployed Agent

```bash
# Stop the service
sudo systemctl stop my-agent

# Edit the embedded config
muti-metroo setup -c /usr/local/bin/my-agent

# Restart the service
sudo systemctl start my-agent
```

## Service Installation with Embedded Config

When installing as a service, embedded binaries are handled specially:

1. The binary is copied to a standard location:
   - Linux/macOS: `/usr/local/bin/<service-name>`
   - Windows: `C:\Program Files\<service-name>\<service-name>.exe`

2. The service definition runs without `-c` flag or `run` command:
   ```bash
   # Traditional service
   ExecStart=/usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml

   # Embedded config service (no arguments needed)
   ExecStart=/usr/local/bin/my-agent
   ```

### Linux (systemd)

```bash
# Wizard creates:
# - /usr/local/bin/my-agent (embedded binary)
# - /etc/systemd/system/my-agent.service

sudo systemctl enable my-agent
sudo systemctl start my-agent
```

### macOS (launchd)

```bash
# Wizard creates:
# - /usr/local/bin/my-agent (embedded binary)
# - /Library/LaunchDaemons/com.my-agent.plist

sudo launchctl load /Library/LaunchDaemons/com.my-agent.plist
```

### Windows

```powershell
# Wizard creates:
# - C:\Program Files\my-agent\my-agent.exe (embedded binary)
# - Windows Service registration

sc start my-agent
```

## Binary Format

The embedded binary format is:

```
[original executable]
[XOR-obfuscated YAML config]
[8-byte config length (little-endian)]
[8-byte magic: "MUTICFG\0"]
```

The XOR obfuscation prevents casual inspection of the config but is **not cryptographic security**. Do not rely on it for protecting secrets - use the management key feature for sensitive data.

## Use Cases

### IoT and Edge Devices

Single-file deployment is ideal for resource-constrained devices:
- Deploy a single executable to edge nodes
- No config files to manage on limited storage
- Pre-configured for immediate operation
- Easy to update config without redeploying binary

### Air-Gapped Environments

For environments without network access during deployment:
- Pre-configure the binary with all settings
- Transfer single file to target
- Run immediately without additional setup

### Simplified Distribution

For distributing pre-configured agents:
- Build once with embedded config
- Distribute single file to remote sites
- No separate config file management

## Backup Configuration

When embedding, the wizard also saves a backup config file:

```
Backup config saved to: ./config.yaml
```

Keep this backup for:
- Documentation of the configuration
- Recovery if the binary is lost
- Reference when editing embedded config

## Security Considerations

### Config Obfuscation

The XOR obfuscation:
- Prevents casual `strings` inspection
- Is NOT encryption - can be reversed
- Should not be relied upon for secrets

### Secrets Management

For sensitive data:
- Use the management key feature for topology encryption
- Store passwords as bcrypt hashes (generated by `muti-metroo hash`)
- Consider environment variable substitution for runtime secrets

### Binary Integrity

After embedding:
- The binary checksum will change
- Re-sign if code signing is required
- Update any integrity verification systems
