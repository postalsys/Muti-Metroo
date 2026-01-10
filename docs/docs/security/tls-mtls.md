---
title: TLS and mTLS
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole securing connections" style={{maxWidth: '180px'}} />
</div>

# TLS and Mutual TLS

Control which agents can connect to your mesh. With basic TLS, agents verify they're connecting to the right server. With mutual TLS (mTLS), both sides verify each other - unauthorized agents can't connect at all.

**Quick decision:**
- Development/testing: TLS without mTLS is fine
- Production: Always enable mTLS - only agents with valid certificates can connect

## TLS Basics

All connections between agents are encrypted with TLS 1.3. Traffic between agents cannot be read by network observers.

## Global TLS Configuration

TLS is configured once in the global `tls:` section and automatically used by all listeners and peer connections:

```yaml
tls:
  ca: "./certs/ca.crt"        # CA for verifying peers and clients
  cert: "./certs/agent.crt"   # Agent's identity certificate
  key: "./certs/agent.key"    # Agent's private key
  mtls: true                  # Enable mutual TLS on listeners
```

## Server Authentication (TLS)

Basic TLS validates the server's certificate. With `mtls: false`, listeners accept any connecting peer:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  mtls: false                  # Don't require client certs

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
    # Uses global CA to validate server, global cert as client cert
```

### What You Get

- You know you're connecting to the right agent (not an imposter)
- Traffic is encrypted between agents
- But: any client with network access can connect to listeners

## Mutual TLS (mTLS)

mTLS requires both sides to present valid certificates. Enable it globally:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  mtls: true                   # Require client certs on listeners
```

With mTLS enabled:
- Listeners require connecting peers to present valid certificates
- The global CA is used to verify client certificates
- Peers automatically use the global agent certificate as their client certificate

### What You Get with mTLS

- Only agents with valid certificates can connect
- Both sides verify each other - no anonymous connections
- Unauthorized agents are rejected before they can do anything
- You control who's in your mesh by controlling who has certificates

## EC-Only Certificates

**Important**: Muti Metroo only accepts EC (Elliptic Curve) certificates. RSA certificates are rejected.

```bash
# Generate EC certificates using the CLI
muti-metroo cert ca --cn "My Mesh CA" -o ./certs
muti-metroo cert agent --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs
```

The only exception is WebSocket connections through a proxy - external servers may use any certificate type since mTLS is not available through proxies.

## Certificate Types

### CA Certificate

Signs all other certificates. Never share the private key:

```bash
muti-metroo cert ca --cn "My Mesh CA" -o ./certs

# Files created:
# ./certs/ca.crt   - Share with all agents
# ./certs/ca.key   - KEEP SECRET!
```

### Agent Certificate

Used for both server and client authentication:

```bash
muti-metroo cert agent --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  --dns "agent1.example.com" \
  --ip "192.168.1.10" \
  -o ./certs
```

Features:
- Extended Key Usage: Server Authentication, Client Authentication
- Used by listeners for server TLS
- Used by peers as client certificate for mTLS

## Configuration Examples

### Development (mTLS Disabled)

```yaml
tls:
  ca: "./dev-certs/ca.crt"
  cert: "./dev-certs/agent.crt"
  key: "./dev-certs/agent.key"
  mtls: false                  # Disable for easier development

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
```

### Production (Full mTLS)

```yaml
tls:
  ca: "/etc/muti-metroo/certs/ca.crt"
  cert: "/etc/muti-metroo/certs/agent.crt"
  key: "/etc/muti-metroo/certs/agent.key"
  mtls: true

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
```

### Per-Listener Override

Disable mTLS for a specific listener:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  mtls: true                   # Global default

listeners:
  # Uses global mTLS setting
  - transport: quic
    address: "0.0.0.0:4433"

  # Override: disable mTLS for this listener
  - transport: h2
    address: "0.0.0.0:8443"
    tls:
      mtls: false
```

### Kubernetes (Inline Certs)

```yaml
tls:
  ca_pem: "${CA_CRT}"
  cert_pem: "${TLS_CRT}"
  key_pem: "${TLS_KEY}"
  mtls: true

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
```

## Certificate Validation

### What's Validated

1. Certificate signature (signed by trusted CA)
2. Validity period (not expired, not future)
3. Subject Alternative Names (hostname/IP matches)
4. Key usage (appropriate for server/client)
5. Key type (must be ECDSA)

### Peer ID Validation

In addition to TLS, Muti Metroo validates the peer's Agent ID:

```yaml
peers:
  - id: "abc123def456..."    # Expected Agent ID
```

This provides defense-in-depth: even if an attacker has a valid certificate, they must also have the correct Agent ID.

## Troubleshooting

### Certificate Not Trusted

```
Error: x509: certificate signed by unknown authority
```

- Verify CA certificate is correct
- Check CA was used to sign the certificate:
  ```bash
  openssl verify -CAfile ca.crt agent.crt
  ```

### Certificate Expired

```
Error: x509: certificate has expired
```

- Check expiration:
  ```bash
  openssl x509 -enddate -noout -in agent.crt
  ```
- Generate new certificate

### Name Mismatch

```
Error: x509: certificate is valid for agent1.example.com, not agent2.example.com
```

- Use correct hostname in peer address
- Generate certificate with correct SANs

### RSA Certificate Rejected

```
Error: certificate must use ECDSA, got RSA
```

- Muti Metroo only accepts EC certificates
- Regenerate using ECDSA:
  ```bash
  muti-metroo cert agent --cn "agent-1" \
    --ca ./certs/ca.crt \
    --ca-key ./certs/ca.key \
    -o ./certs
  ```

### Client Certificate Required

```
Error: tls: client didn't provide a certificate
```

- The listener has mTLS enabled
- Connecting peer must have a valid certificate configured
- Check global `tls.cert` and `tls.key` are set

## Best Practices

1. **Always use mTLS in production**
2. **Use EC certificates only** (RSA is not supported)
3. **Protect CA private key** (HSM, vault, or encrypted storage)
4. **Use short certificate validity** (90-365 days)
5. **Automate certificate rotation**
6. **Include all necessary SANs** when generating certs
7. **Monitor certificate expiration**
8. **Revoke compromised certificates** immediately

## Next Steps

- [Authentication](/security/authentication) - Additional authentication layers
- [TLS Certificates](/configuration/tls-certificates) - Configuration reference
- [Best Practices](/security/best-practices) - Production hardening
