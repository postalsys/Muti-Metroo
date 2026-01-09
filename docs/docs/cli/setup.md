---
title: setup
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-idea.png" alt="Mole setup wizard" style={{maxWidth: '180px'}} />
</div>

# muti-metroo setup

Interactive setup wizard for configuring Muti Metroo.

## Usage

```bash
# New configuration
muti-metroo setup

# Edit embedded configuration in existing binary
muti-metroo setup -c /path/to/embedded-binary
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file or binary with embedded config |

## What It Does

Guides you through:

1. Basic configuration (data directory, config file)
2. Agent role selection (ingress, transit, exit)
3. Network configuration (transport, listen address)
4. TLS certificate setup (generate, paste, or use files)
5. Peer connections
6. SOCKS5 configuration (if ingress)
7. Exit routes (if exit)
8. Advanced options (logging, HTTP API)
9. Configuration delivery (save to file or embed in binary)
10. Service installation (if root/admin)

## Output

Generates complete `config.yaml` and optionally:
- TLS certificates
- Agent identity
- Embedded binary (single-file deployment)
- Systemd service file (Linux)
- Windows Service registration (Windows)

## Embedded Configuration

The wizard can embed configuration directly into the binary for single-file deployments:

```bash
# During setup, choose "Embed in binary" when prompted
muti-metroo setup

# Edit existing embedded config
muti-metroo setup -c /usr/local/bin/my-agent
```

See [Embedded Configuration](/deployment/embedded-config) for details.

See [Interactive Setup](/getting-started/interactive-setup) for detailed guide.
