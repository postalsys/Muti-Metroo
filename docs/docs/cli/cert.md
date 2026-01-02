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
muti-metroo cert ca -n <name> [-o <output-dir>] [-d <days>]
```

**Flags:**
- `-n, --cn`: CA common name (default: "Muti Metroo CA")
- `-o, --out`: Output directory (default: ./certs)
- `-d, --days`: Validity period in days (default: 365)

**Output:**
- `ca.crt`: CA certificate
- `ca.key`: CA private key

### cert agent

Generate agent/peer certificate.

```bash
muti-metroo cert agent -n <name> [--dns <hostnames>] [--ip <ips>] [-o <output>] [-d <days>]
```

**Flags:**
- `-n, --cn`: Certificate common name (required)
- `--dns`: Additional DNS names (comma-separated)
- `--ip`: Additional IP addresses (comma-separated)
- `-o, --out`: Output directory (default: ./certs)
- `-d, --days`: Validity period in days (default: 90)
- `--ca`: CA certificate path (default: ./certs/ca.crt)
- `--ca-key`: CA key path (default: ./certs/ca.key)

**Output:**
- `<name>.crt`: Agent certificate
- `<name>.key`: Agent private key

### cert client

Generate client-only certificate.

```bash
muti-metroo cert client -n <name> [-o <output>] [-d <days>]
```

**Flags:**
- `-n, --cn`: Certificate common name (required)
- `-o, --out`: Output directory (default: ./certs)
- `-d, --days`: Validity period in days (default: 90)
- `--ca`: CA certificate path (default: ./certs/ca.crt)
- `--ca-key`: CA key path (default: ./certs/ca.key)

**Output:**
- `<name>.crt`: Client certificate
- `<name>.key`: Client private key

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
