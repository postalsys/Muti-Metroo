---
title: End-to-End Encryption
sidebar_position: 2
---

# End-to-End Encryption

Muti Metroo provides automatic end-to-end encryption for all stream data. Only the ingress (entry) and exit agents can read the payload - transit agents cannot decrypt it.

## Overview

All stream data is encrypted automatically using modern cryptography:

- **Key Exchange**: X25519 elliptic curve Diffie-Hellman
- **Encryption**: ChaCha20-Poly1305 authenticated encryption
- **Forward Secrecy**: Each stream uses unique ephemeral keys

## Security Properties

| Property | Description |
|----------|-------------|
| **Confidentiality** | Only ingress and exit can read stream data |
| **Integrity** | Tampering is detected and rejected |
| **Forward Secrecy** | Each stream uses ephemeral keys |
| **Transit Opacity** | Transit agents see only encrypted data |

## What Is Encrypted

| Data | Encrypted | Notes |
|------|-----------|-------|
| Stream payload | Yes | All application data |
| Destination address/port | No | Required for routing |

## No Configuration Required

End-to-end encryption is enabled automatically. There is no configuration to set up - it just works.

### Key Generation

Each agent automatically generates an X25519 keypair on first start:

| File | Purpose | Permissions |
|------|---------|-------------|
| `{data_dir}/agent_key` | Private key (never shared) | 0600 (owner only) |
| `{data_dir}/agent_key.pub` | Public key (distributed to peers) | 0644 (world readable) |

The keypair is persistent - once generated, it's reused on subsequent starts. The public key is automatically distributed to other agents via NodeInfo advertisements, so peers can encrypt data destined for this agent.

### Stream Encryption Flow

When a stream is opened:
1. Ingress and exit agents exchange ephemeral public keys
2. Both derive a shared secret using X25519 ECDH
3. All stream data is encrypted with ChaCha20-Poly1305
4. Transit agents forward encrypted data unchanged

## Performance Impact

| Metric | Impact |
|--------|--------|
| CPU | Minimal (~5-10% for encryption) |
| Latency | Negligible |
| Bandwidth | +28 bytes per frame |

ChaCha20-Poly1305 is highly optimized and runs at several GB/s on modern CPUs.

## E2E vs Management Encryption

Muti Metroo uses two separate encryption systems:

| Feature | E2E Encryption | Management Encryption |
|---------|----------------|----------------------|
| **Purpose** | Protect stream payload data | Protect mesh topology metadata |
| **What's encrypted** | Application data in streams | NodeInfo (hostnames, IPs, OS) |
| **Key type** | Per-agent persistent keypair | Shared across all agents |
| **Automatic** | Yes, always on | No, requires configuration |
| **Algorithm** | X25519 + ChaCha20-Poly1305 | X25519 + ChaCha20-Poly1305 (sealed boxes) |

Both systems use the same cryptographic primitives but serve different purposes. E2E encryption protects your traffic; management encryption protects your infrastructure topology.

See [Management Keys](/red-team/management-keys) for management encryption details.

## Threat Protection

### Protected Against

- Passive eavesdropping at transit nodes
- Compromised transit agents reading your data
- Replay attacks
- Message tampering

### Not Protected Against

- Compromised ingress or exit agent (secure your endpoints)
- Traffic analysis (timing, volume patterns)
- Metadata leakage (destination is visible for routing)

## Troubleshooting

### Decryption Failures

If streams fail with decryption errors:

1. **Version mismatch**: Ensure all agents are running the same version
2. **Corrupted frames**: Check network reliability
3. **Clock skew**: Verify system time is synchronized

### Key Issues

```bash
# Verify keypair exists
ls -la {data_dir}/agent_key*

# Regenerate if corrupted (will change agent identity)
rm {data_dir}/agent_key*
muti-metroo init -d {data_dir}
```

## Next Steps

- [TLS/mTLS Configuration](tls-mtls) - Transport-layer security between peers
- [Authentication](authentication) - SOCKS5 and shell authentication
