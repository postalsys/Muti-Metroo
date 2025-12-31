---
title: cert
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole managing certificates" style={{maxWidth: '180px'}} />
</div>

# muti-metroo cert

Certificate management commands.

## Subcommands

### cert ca

Generate Certificate Authority.

```bash
muti-metroo cert ca -n <name> [-o <output-dir>] [--days <days>]
```

**Flags:**
- `-n, --name`: CA common name (required)
- `-o, --output`: Output directory (default: ./certs)
- `--days`: Validity period in days (default: 365)

**Output:**
- `ca.crt`: CA certificate
- `ca.key`: CA private key

### cert agent

Generate agent/peer certificate.

```bash
muti-metroo cert agent -n <name> [--dns <hostname>] [--ip <ip>] [-o <output>]
```

**Flags:**
- `-n, --name`: Certificate common name (required)
- `--dns`: DNS name (can be repeated)
- `--ip`: IP address (can be repeated)
- `-o, --output`: Output directory (default: ./certs)
- `--ca-cert`: CA certificate path (default: ./certs/ca.crt)
- `--ca-key`: CA key path (default: ./certs/ca.key)

**Output:**
- `agent.crt`: Agent certificate
- `agent.key`: Agent private key

### cert client

Generate client-only certificate.

```bash
muti-metroo cert client -n <name> [-o <output>]
```

**Flags:**
- `-n, --name`: Certificate common name (required)
- `-o, --output`: Output directory (default: ./certs)
- `--ca-cert`: CA certificate path
- `--ca-key`: CA key path

**Output:**
- `client.crt`: Client certificate
- `client.key`: Client private key

### cert info

Display certificate information.

```bash
muti-metroo cert info <cert-file>
```

**Example output:**
```
Subject: CN=My Agent
Issuer: CN=My CA
Not Before: 2025-01-01 00:00:00
Not After: 2026-01-01 00:00:00
DNS Names: agent1.example.com
IP Addresses: 192.168.1.10
```

## Examples

```bash
# Generate CA
muti-metroo cert ca -n "Mesh CA"

# Generate agent cert
muti-metroo cert agent -n "agent-1"     --dns agent1.example.com     --ip 192.168.1.10

# Generate client cert
muti-metroo cert client -n "admin"

# View cert info
muti-metroo cert info ./certs/agent.crt
```
