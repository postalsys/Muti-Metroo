---
title: Security Overview
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole inspecting security" style={{maxWidth: '180px'}} />
</div>

# Security Overview

Muti Metroo is designed with security as a core principle. This guide covers the security model and best practices.

## Security Model

### End-to-End Encryption

All stream data is encrypted end-to-end between ingress and exit agents:

- **X25519 Key Exchange**: Ephemeral keys per stream for forward secrecy
- **ChaCha20-Poly1305**: Authenticated encryption prevents tampering
- **Transit Opacity**: Transit agents cannot decrypt payload data
- **Key Derivation**: HKDF-SHA256 with stream context

See [End-to-End Encryption](/security/e2e-encryption) for details.

### Transport Security

All peer connections use TLS 1.3:

- **Encryption**: All traffic encrypted in transit
- **Authentication**: Server certificates validated
- **Mutual TLS**: Optional client certificate authentication
- **Perfect Forward Secrecy**: Session keys derived per connection

### Authentication Layers

| Layer | Mechanism | Purpose |
|-------|-----------|---------|
| E2E | X25519 + ChaCha20-Poly1305 | Stream data encryption |
| TLS | Certificates | Peer authentication |
| mTLS | Client certs | Mutual authentication |
| SOCKS5 | Username/password | Client authentication |
| Shell | bcrypt password | Command authorization |
| File Transfer | bcrypt password | Transfer authorization |

### Authorization

- **Route-based ACL**: Exit only allows configured CIDRs
- **Shell whitelist**: Only whitelisted commands allowed
- **File path restrictions**: Only allowed paths accessible

## Threat Model

### What Muti Metroo Protects Against

| Threat | Protection |
|--------|------------|
| Eavesdropping | TLS + E2E encryption |
| Compromised transit | E2E encryption (transit cannot decrypt) |
| Replay attacks | Nonce-based encryption |
| Data tampering | Authenticated encryption (Poly1305) |
| Man-in-the-middle | Certificate validation |
| Unauthorized peers | mTLS authentication |
| Unauthorized clients | SOCKS5 authentication |
| Unauthorized commands | Shell whitelist + password |
| Resource exhaustion | Connection limits |

### What Muti Metroo Does NOT Protect Against

| Threat | Mitigation |
|--------|------------|
| Compromised CA | Secure CA key management |
| Compromised host | Host security hardening |
| Traffic analysis | Use VPN/Tor if needed |
| Insider threat | Audit logging, monitoring |

## Security Checklist

### Minimum Security

- [ ] TLS certificates generated and deployed
- [ ] Certificate CA key secured
- [ ] SOCKS5 bound to localhost or authenticated
- [ ] Exit routes restricted to needed CIDRs

### Recommended Security

- [ ] Mutual TLS enabled
- [ ] SOCKS5 authentication enabled
- [ ] Shell disabled or password-protected
- [ ] File transfer disabled or restricted
- [ ] Monitoring and alerting configured
- [ ] Regular certificate rotation

### Production Security

- [ ] All of the above
- [ ] CA key in HSM or secure vault
- [ ] Network segmentation
- [ ] Intrusion detection
- [ ] Audit logging
- [ ] Incident response plan

## Quick Hardening

### Disable Unnecessary Features

```yaml
# Disable shell if not needed
shell:
  enabled: false

# Disable file transfer if not needed
file_transfer:
  enabled: false
```

### Restrict SOCKS5 Access

```yaml
# Localhost only + authentication
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user"
        password_hash: "$2a$12$..."    # Use cost 12+
```

### Restrict Exit Routes

```yaml
# Only allow specific destinations
exit:
  enabled: true
  routes:
    - "10.0.1.0/24"    # Specific subnet, not 0.0.0.0/0
```

### Enable mTLS

```yaml
# Require client certificates
tls:
  ca: "./certs/ca.crt"           # CA for verifying client certs
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  mtls: true                     # Require valid client certificates

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    # Uses global TLS settings with mTLS enabled
```

## Security Topics

| Topic | Description |
|-------|-------------|
| [E2E Encryption](/security/e2e-encryption) | Stream payload encryption |
| [TLS/mTLS](/security/tls-mtls) | Certificate-based security |
| [Authentication](/security/authentication) | Client and shell authentication |
| [Access Control](/security/access-control) | Route and command restrictions |
| [Best Practices](/security/best-practices) | Production hardening guide |

## Reporting Security Issues

If you discover a security vulnerability:

1. Do NOT open a public issue
2. Contact the maintainers privately
3. Provide detailed reproduction steps
4. Allow reasonable time for fix before disclosure

## Next Steps

- [End-to-End Encryption](/security/e2e-encryption)
- [TLS/mTLS Configuration](/security/tls-mtls)
- [Authentication](/security/authentication)
- [Best Practices](/security/best-practices)
