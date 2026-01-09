# TLS Certificates

All peer connections in Muti Metroo require TLS encryption. This chapter covers certificate generation and management.

## Certificate Types

| Type | Command | Server Auth | Client Auth | Use Case |
|------|---------|:-----------:|:-----------:|----------|
| CA | `cert ca` | N/A | N/A | Sign other certificates |
| Agent | `cert agent` | Yes | Yes | Listeners and peer connections |
| Client | `cert client` | No | Yes | Client-only connections |

## Generate Certificate Authority

Create a CA to sign all certificates in your mesh:

```bash
muti-metroo cert ca --cn "Mesh CA" -o ./certs
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--cn` | "Muti Metroo CA" | Common name for the CA |
| `-o, --out` | ./certs | Output directory |
| `--days` | 365 | Validity period in days |

**Output files:**
- `ca.crt` - CA certificate (share with all agents)
- `ca.key` - CA private key (keep secure!)

**Security**: Keep `ca.key` secure. Anyone with access can create valid certificates for your mesh.

## Generate Agent Certificate

Create a certificate for an agent (supports both server and client authentication):

```bash
muti-metroo cert agent --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  --dns agent1.example.com \
  --ip 192.168.1.10 \
  -o ./certs
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--cn` | (required) | Common name for the certificate |
| `--dns` | | Additional DNS names (comma-separated) |
| `--ip` | | Additional IP addresses (comma-separated) |
| `-o, --out` | ./certs | Output directory |
| `--days` | 90 | Validity period |
| `--ca` | ./certs/ca.crt | CA certificate path |
| `--ca-key` | ./certs/ca.key | CA private key path |

**Output files:**
- `<name>.crt` - Agent certificate (named after common name)
- `<name>.key` - Agent private key

## Generate Client Certificate

Create a client-only certificate (cannot be used for server authentication):

```bash
muti-metroo cert client --cn "admin" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs
```

## View Certificate Information

Display detailed information about a certificate:

```bash
muti-metroo cert info ./certs/agent-1.crt
```

**Example output:**

```
Certificate: ./certs/agent-1.crt

Subject:      CN=agent-1,O=Muti Metroo
Issuer:       CN=Mesh CA,O=Muti Metroo
Serial:       1a2b3c4d5e6f...
Fingerprint:  sha256:ab12cd34...
Is CA:        false
Not Before:   2026-01-01T00:00:00Z
Not After:    2026-04-01T00:00:00Z
Status:       Valid (89 days left)
DNS Names:    agent-1, localhost, agent1.example.com
IP Addresses: 127.0.0.1, ::1, 192.168.1.10
Key Usage:    KeyEncipherment, DigitalSignature
Ext Key Usage: ServerAuth, ClientAuth
```

## Configuration

### Global TLS Configuration

Configure TLS settings at the top level to apply to all listeners and peers:

```yaml
tls:
  ca: "./certs/ca.crt"          # CA certificate
  cert: "./certs/agent.crt"     # Agent certificate
  key: "./certs/agent.key"      # Private key
  mtls: false                   # Enable mutual TLS
```

### Per-Listener Override

Override TLS settings for specific listeners:

```yaml
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/listener-specific.crt"
      key: "./certs/listener-specific.key"
```

### Per-Peer Override

Override TLS settings for specific peer connections:

```yaml
peers:
  - id: "abc123..."
    transport: quic
    address: "192.168.1.10:4433"
    tls:
      ca: "./certs/other-ca.crt"
```

## Mutual TLS (mTLS)

Enable mTLS to require client certificates on listeners:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  mtls: true                    # Require client certificates
```

With mTLS enabled:
- Listeners require connecting peers to present a valid client certificate
- Peers must have a certificate signed by a trusted CA

## Inline Certificates

For scenarios where file-based certificates are impractical, use inline PEM:

```yaml
tls:
  ca_pem: |
    -----BEGIN CERTIFICATE-----
    MIICpDCCAYwCCQC...
    -----END CERTIFICATE-----
  cert_pem: |
    -----BEGIN CERTIFICATE-----
    MIICpDCCAYwCCQC...
    -----END CERTIFICATE-----
  key_pem: |
    -----BEGIN PRIVATE KEY-----
    MIIEvgIBADANBg...
    -----END PRIVATE KEY-----
```

Inline PEM takes precedence over file paths if both are specified.

## Certificate Rotation

To rotate certificates without downtime:

1. Generate new certificates with the same CA
2. Update configuration with new certificate paths
3. Restart the agent

```bash
# Generate new certificate
muti-metroo cert agent --cn "agent-1-new" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs

# Update config.yaml to use new certificate
# Restart agent
```

## Best Practices

1. **Protect CA key**: Store `ca.key` separately from agents
2. **Use short validity**: 90 days for agent certs, rotate regularly
3. **Include SANs**: Add DNS names and IPs that agents will use
4. **Separate CAs**: Use different CAs for different environments
5. **Monitor expiration**: Check certificate validity regularly
