---
title: TLS and mTLS
sidebar_position: 2
---

# TLS and Mutual TLS

Secure your mesh with TLS encryption and mutual authentication.

## TLS Basics

All Muti Metroo peer connections use TLS 1.3:

- **Encryption**: AES-256-GCM or ChaCha20-Poly1305
- **Key Exchange**: ECDHE with X25519 or P-256
- **Authentication**: RSA or ECDSA certificates
- **Forward Secrecy**: New keys per session

## Server Authentication (TLS)

Basic TLS validates the server's certificate:

```yaml
# Listener (server)
listeners:
  - transport: quic
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

# Peer connection (client)
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"     # Validate server cert
```

### What This Provides

- Server identity verified
- Client can trust it's connecting to the right agent
- Traffic encrypted

### What This Does NOT Provide

- No validation of client identity
- Any client can connect (if they can reach the port)

## Mutual TLS (mTLS)

mTLS requires both sides to present valid certificates:

```yaml
# Listener (server) - require client certs
listeners:
  - transport: quic
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"    # Validate client certs

# Peer connection (client) - present client cert
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"     # Client certificate
      key: "./certs/client.key"      # Client key
```

### What mTLS Provides

- Mutual authentication (both sides verified)
- Only authorized peers can connect
- Strong defense against unauthorized access
- Certificate-based access control

## Certificate Types

### CA Certificate

Signs all other certificates. Never share the private key:

```bash
./build/muti-metroo cert ca -n "My Mesh CA" -o ./certs

# Files created:
# ./certs/ca.crt   - Share with all agents
# ./certs/ca.key   - KEEP SECRET!
```

### Agent Certificate

For listeners (server authentication):

```bash
./build/muti-metroo cert agent -n "agent-1" \
  --dns "agent1.example.com" \
  --ip "192.168.1.10"
```

Features:
- Extended Key Usage: Server Authentication, Client Authentication
- Can be used for both server and client

### Client Certificate

For peer connections (client authentication):

```bash
./build/muti-metroo cert client -n "peer-client"
```

Features:
- Extended Key Usage: Client Authentication only
- Cannot be used for server authentication

## Certificate Requirements

### Subject Alternative Names (SANs)

Always include SANs for server certificates:

```bash
./build/muti-metroo cert agent -n "agent-1" \
  --dns "agent1.example.com" \
  --dns "agent1.internal" \
  --ip "192.168.1.10" \
  --ip "10.0.0.10"
```

Without proper SANs, TLS validation will fail.

### Key Size

- **RSA**: Minimum 2048 bits, recommended 4096 for CA
- **ECDSA**: P-256 or P-384

### Validity Period

- **CA**: 1-5 years
- **Agent/Client**: 90-365 days
- Shorter validity = more frequent rotation = better security

## Configuration Examples

### Development (TLS Only)

```yaml
# Development - TLS but no client certs
listeners:
  - transport: quic
    tls:
      cert: "./dev-certs/agent.crt"
      key: "./dev-certs/agent.key"

peers:
  - id: "..."
    tls:
      ca: "./dev-certs/ca.crt"
```

### Production (Full mTLS)

```yaml
# Production - mutual TLS
listeners:
  - transport: quic
    tls:
      cert: "/etc/muti-metroo/certs/agent.crt"
      key: "/etc/muti-metroo/certs/agent.key"
      client_ca: "/etc/muti-metroo/certs/ca.crt"

peers:
  - id: "..."
    tls:
      ca: "/etc/muti-metroo/certs/ca.crt"
      cert: "/etc/muti-metroo/certs/client.crt"
      key: "/etc/muti-metroo/certs/client.key"
```

### Kubernetes (Inline Certs)

```yaml
listeners:
  - transport: quic
    tls:
      cert_pem: "${TLS_CRT}"
      key_pem: "${TLS_KEY}"
      client_ca_pem: "${CA_CRT}"
```

## Certificate Validation

### What's Validated

1. Certificate signature (signed by trusted CA)
2. Validity period (not expired, not future)
3. Subject Alternative Names (hostname/IP matches)
4. Key usage (appropriate for server/client)

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

### Client Certificate Required

```
Error: tls: client didn't provide a certificate
```

- Configure client certificate in peer config:
  ```yaml
  peers:
    - tls:
        cert: "./certs/client.crt"
        key: "./certs/client.key"
  ```

## Best Practices

1. **Always use mTLS in production**
2. **Protect CA private key** (HSM, vault, or encrypted storage)
3. **Use short certificate validity** (90-365 days)
4. **Automate certificate rotation**
5. **Include all necessary SANs** when generating certs
6. **Monitor certificate expiration**
7. **Revoke compromised certificates** immediately

## Next Steps

- [Authentication](authentication) - Additional authentication layers
- [TLS Certificates](../configuration/tls-certificates) - Configuration reference
- [Best Practices](best-practices) - Production hardening
