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
muti-metroo setup
```

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
9. Service installation (if root/admin)

## Output

Generates complete `config.yaml` and optionally:
- TLS certificates
- Agent identity
- Systemd service file (Linux)
- Windows Service registration (Windows)

See [Interactive Setup](/getting-started/interactive-setup) for detailed guide.
