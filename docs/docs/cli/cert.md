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
muti-metroo cert ca [-n <name>] [-o <output-dir>] [-d <days>]
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--cn` | `-n` | "Muti Metroo CA" | Common name for the CA |
| `--out` | `-o` | ./certs | Output directory |
| `--days` | `-d` | 365 | Validity period in days |

**Output:**
- `ca.crt`: CA certificate
- `ca.key`: CA private key (keep secure!)

### cert agent

Generate agent/peer certificate. The certificate can be used for both server authentication (listeners) and client authentication (peer connections with mTLS).

```bash
muti-metroo cert agent -n <name> [--dns <hostnames>] [--ip <ips>] [-o <output>] [-d <days>]
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--cn` | `-n` | (required) | Common name for the certificate |
| `--dns` | | | Additional DNS names (comma-separated) |
| `--ip` | | | Additional IP addresses (comma-separated) |
| `--out` | `-o` | ./certs | Output directory |
| `--days` | `-d` | 90 | Validity period in days |
| `--ca` | | ./certs/ca.crt | CA certificate path |
| `--ca-key` | | ./certs/ca.key | CA private key path |

**Output:**
- `<name>.crt`: Agent certificate (named after common name)
- `<name>.key`: Agent private key

### cert client

Generate client-only certificate. This certificate can only be used for client authentication (connecting to listeners), not for server authentication.

```bash
muti-metroo cert client -n <name> [-o <output>] [-d <days>]
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--cn` | `-n` | (required) | Common name for the certificate |
| `--out` | `-o` | ./certs | Output directory |
| `--days` | `-d` | 90 | Validity period in days |
| `--ca` | | ./certs/ca.crt | CA certificate path |
| `--ca-key` | | ./certs/ca.key | CA private key path |

**Output:**
- `<name>.crt`: Client certificate (named after common name)
- `<name>.key`: Client private key

### cert info

Display detailed information about a certificate file.

```bash
muti-metroo cert info <cert-file>
```

**Example output:**
```
Certificate: ./certs/agent-1.crt

Subject:      CN=agent-1,O=Muti Metroo
Issuer:       CN=Mesh CA,O=Muti Metroo
Serial:       1a2b3c4d5e6f...
Fingerprint:  sha256:ab12cd34...
Is CA:        false
Not Before:   2025-01-01T00:00:00Z
Not After:    2025-04-01T00:00:00Z
Status:       Valid (89 days left)
DNS Names:    agent-1, localhost, agent1.example.com
IP Addresses: 127.0.0.1, ::1, 192.168.1.10
Key Usage:    KeyEncipherment, DigitalSignature
Ext Key Usage: ServerAuth, ClientAuth
```

## Examples

```bash
# Generate CA
muti-metroo cert ca -n "Mesh CA"

# Generate agent cert with DNS names and IP addresses
muti-metroo cert agent -n "agent-1" \
  --dns agent1.example.com \
  --ip 192.168.1.10

# Generate client cert
muti-metroo cert client -n "admin"

# View cert info
muti-metroo cert info ./certs/agent-1.crt
```

## Certificate Types

| Type | Command | Server Auth | Client Auth | Use Case |
|------|---------|-------------|-------------|----------|
| CA | `cert ca` | N/A | N/A | Sign other certificates |
| Agent | `cert agent` | Yes | Yes | Listeners and peer connections |
| Client | `cert client` | No | Yes | Client-only connections |
