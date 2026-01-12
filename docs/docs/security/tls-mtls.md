---
title: TLS and mTLS
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole securing connections" style={{maxWidth: '180px'}} />
</div>

# TLS and Mutual TLS

## Security Model

Muti Metroo uses a **layered security model**:

1. **End-to-End Encryption (Primary)**: X25519 key exchange + ChaCha20-Poly1305 encrypts all stream data. Transit agents cannot decrypt traffic - only ingress and exit agents can read the payload.

2. **Transport TLS (Defense-in-depth)**: TLS 1.3 encrypts the transport layer. This provides an additional encryption layer but is not the primary security mechanism.

Because of the E2E encryption layer, **TLS certificate verification is optional by default**. Agents auto-generate self-signed certificates and don't verify peer certificates. This allows quick deployment without PKI setup while maintaining strong security guarantees.

**Quick decision:**
- Most deployments: Default settings are secure (E2E encryption protects traffic)
- High-trust/compliance: Enable `strict: true` for TLS verification, `mtls: true` for mutual authentication

## TLS Basics

All connections between agents are encrypted with TLS 1.3, even with default settings. The difference is whether certificates are verified:

| Mode | Certificate Source | Verification | Use Case |
|------|-------------------|--------------|----------|
| Default | Auto-generated | None | Quick deployment, development |
| Strict | User-provided | CA-based | Production, compliance |
| Strict + mTLS | User-provided | Mutual | Zero-trust, high-security |

## Default Behavior (No Verification)

By default, Muti Metroo:
- Auto-generates self-signed ECDSA certificates on startup
- Does NOT verify peer certificates
- Traffic is still encrypted (TLS 1.3) and protected by E2E encryption

```yaml
# Minimal config - TLS handled automatically
listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
```

### Why Default No-Verification is Safe

1. **E2E Encryption**: Stream data is encrypted with ChaCha20-Poly1305 using keys derived from X25519 exchange. Even if a MITM intercepts TLS, they cannot decrypt stream content.

2. **Agent ID Verification**: Peers verify each other's Agent ID during handshake. This is independent of TLS certificates.

3. **Double Encryption**: Traffic is encrypted twice - once at the E2E layer, once at TLS. A MITM would need to break both.

## Strict Mode (CA Verification)

Enable `strict: true` to verify peer certificates against a CA:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  strict: true                 # Enable certificate verification

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
```

With strict mode:
- Peer certificates are verified against the configured CA
- Invalid certificates are rejected at the TLS layer
- Requires all peers to have CA-signed certificates

### When to Use Strict Mode

- **Defense-in-depth**: Additional validation beyond E2E encryption
- **Network access control**: Reject connections from agents without valid certs
- **Compliance**: Environments requiring PKI-based authentication
- **Detection of compromise**: TLS verification can detect MITM attempts earlier

## Mutual TLS (mTLS)

mTLS requires both sides to present valid certificates:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  strict: true
  mtls: true                   # Require client certificates

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
```

With mTLS enabled:
- Listeners require connecting peers to present valid certificates
- The CA is used to verify client certificates
- Unauthorized agents are rejected before handshake completes

### Benefits of mTLS

- **Mutual authentication**: Both sides verify each other
- **Network-level access control**: Only agents with valid certs can connect
- **Early rejection**: Unauthorized connections rejected at TLS, before protocol handshake
- **Zero-trust compliance**: Required for many security frameworks

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

### Development (Default - No Verification)

```yaml
agent:
  id: "auto"
  data_dir: "./data"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

# TLS auto-configured, no verification
# Still secure due to E2E encryption
```

### Strict TLS (CA Verification)

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  strict: true

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
```

### Production (Full mTLS)

```yaml
tls:
  ca: "/etc/muti-metroo/certs/ca.crt"
  cert: "/etc/muti-metroo/certs/agent.crt"
  key: "/etc/muti-metroo/certs/agent.key"
  strict: true
  mtls: true

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "abc123..."
    transport: quic
    address: "peer.example.com:4433"
```

### Per-Peer Strict Override

Enable strict verification for specific peers:

```yaml
tls:
  # Global: no verification
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"

peers:
  # Default: no verification
  - id: "abc123..."
    transport: quic
    address: "internal.example.com:4433"

  # Override: strict verification for this peer
  - id: "def456..."
    transport: quic
    address: "external.example.com:4433"
    tls:
      ca: "./certs/external-ca.crt"
      strict: true
```

### Mixed TLS Environments (Public Proxy)

When using strict TLS with your internal CA but connecting through a public proxy (like nginx with Let's Encrypt), disable strict verification for that specific peer:

```yaml
tls:
  # Global: strict verification with internal CA
  ca_pem: |
    -----BEGIN CERTIFICATE-----
    ... your internal CA ...
    -----END CERTIFICATE-----
  cert_pem: |
    -----BEGIN CERTIFICATE-----
    ... agent cert signed by internal CA ...
    -----END CERTIFICATE-----
  key_pem: |
    -----BEGIN PRIVATE KEY-----
    ... agent private key ...
    -----END PRIVATE KEY-----
  strict: true

peers:
  # Internal peers: use global strict settings
  - id: "abc123..."
    transport: quic
    address: "192.168.1.10:4433"
    # No tls override = verified against internal CA

  # WebSocket through nginx with Let's Encrypt
  - id: "def456..."
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    tls:
      strict: false   # Skip TLS verification for public proxy
```

**Why this is safe:** The E2E encryption layer (X25519 + ChaCha20-Poly1305) protects all traffic regardless of TLS verification. Even if TLS is not verified, the actual mesh traffic remains encrypted end-to-end.

### Kubernetes (Inline Certs)

```yaml
tls:
  ca_pem: "${CA_CRT}"
  cert_pem: "${TLS_CRT}"
  key_pem: "${TLS_KEY}"
  strict: true
  mtls: true

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
```

## Certificate Validation (Strict Mode)

### What's Validated

1. Certificate signature (signed by trusted CA)
2. Validity period (not expired, not future)
3. Subject Alternative Names (hostname/IP matches)
4. Key usage (appropriate for server/client)
5. Key type (must be ECDSA)

### Additional Protections

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

- You likely have `strict: true` enabled
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

1. **Start with defaults**: E2E encryption provides strong security without PKI
2. **Enable strict mode for production**: When defense-in-depth is needed
3. **Use mTLS for zero-trust**: When network-level authentication required
4. **Use EC certificates only**: RSA is not supported
5. **Protect CA private key**: HSM, vault, or encrypted storage
6. **Use short certificate validity**: 90-365 days
7. **Automate certificate rotation**
8. **Monitor certificate expiration**

## Next Steps

- [Authentication](/security/authentication) - Additional authentication layers
- [TLS Certificates](/configuration/tls-certificates) - Configuration reference
- [Best Practices](/security/best-practices) - Production hardening
