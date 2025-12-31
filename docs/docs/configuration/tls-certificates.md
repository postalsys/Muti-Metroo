---
title: TLS Certificates
sidebar_position: 7
---

# TLS Certificate Configuration

All peer connections in Muti Metroo use TLS for encryption and authentication. This guide covers certificate generation and configuration.

## Overview

Muti Metroo uses a PKI (Public Key Infrastructure) model:

1. **Certificate Authority (CA)**: Signs all certificates
2. **Agent Certificates**: Used by listeners for server auth
3. **Client Certificates**: Optional, for mutual TLS (mTLS)

## Generating Certificates

### Using the CLI

```bash
# Generate CA (do once, share across mesh)
./build/muti-metroo cert ca -n "My Mesh CA" -o ./certs

# Generate agent certificate
./build/muti-metroo cert agent -n "agent-1" \
  --dns "agent1.example.com" \
  --ip "192.168.1.10" \
  -o ./certs

# Generate client certificate (for mTLS)
./build/muti-metroo cert client -n "client-1" -o ./certs

# View certificate info
./build/muti-metroo cert info ./certs/agent.crt
```

### Using OpenSSL

```bash
# Generate CA key and certificate
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 365 \
  -out ca.crt -subj "/CN=My Mesh CA"

# Generate agent key and CSR
openssl genrsa -out agent.key 2048
openssl req -new -key agent.key -out agent.csr \
  -subj "/CN=agent-1"

# Sign agent certificate
openssl x509 -req -in agent.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out agent.crt -days 365 -sha256
```

## Certificate Types

### CA Certificate

Signs all other certificates. Share the public part (`ca.crt`) with all agents.

```
./certs/ca.crt    # Public - share with everyone
./certs/ca.key    # Private - keep secure!
```

### Agent Certificate

Used by listeners for TLS handshake:

```yaml
listeners:
  - transport: quic
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
```

### Client Certificate

Used for mutual TLS when connecting to peers:

```yaml
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"
      key: "./certs/client.key"
```

## Configuration Options

### File-Based Certificates

Standard approach using file paths:

```yaml
listeners:
  - transport: quic
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"   # For mTLS

peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"    # For mTLS
      key: "./certs/client.key"     # For mTLS
```

### Inline PEM Certificates

Embed certificates directly in config (useful for Kubernetes secrets):

```yaml
listeners:
  - transport: quic
    tls:
      cert_pem: |
        -----BEGIN CERTIFICATE-----
        MIIBkTCB+wIJAKi...
        -----END CERTIFICATE-----
      key_pem: |
        -----BEGIN PRIVATE KEY-----
        MIIEvQIBADANBg...
        -----END PRIVATE KEY-----
      client_ca_pem: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
```

Inline PEM takes precedence over file paths.

### Environment Variables

Use environment variables for secrets:

```yaml
listeners:
  - transport: quic
    tls:
      cert_pem: "${TLS_CERT}"
      key_pem: "${TLS_KEY}"
      client_ca_pem: "${TLS_CA}"
```

## Mutual TLS (mTLS)

mTLS requires both sides to present certificates:

### Listener (Server Side)

```yaml
listeners:
  - transport: quic
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
      client_ca: "./certs/ca.crt"   # Require valid client cert
```

### Peer (Client Side)

```yaml
peers:
  - id: "..."
    tls:
      ca: "./certs/ca.crt"
      cert: "./certs/client.crt"    # Present client cert
      key: "./certs/client.key"
```

### Benefits of mTLS

- Mutual authentication (both sides verified)
- Prevents unauthorized connections
- Defense against man-in-the-middle attacks
- Required for zero-trust environments

## Certificate Requirements

### Agent Certificates

Must include:
- Extended Key Usage: Server Authentication, Client Authentication
- Subject Alternative Names (SANs) for IP and DNS names

```bash
./build/muti-metroo cert agent -n "agent-1" \
  --dns "agent1.example.com" \
  --dns "agent1.internal" \
  --ip "192.168.1.10" \
  --ip "10.0.0.10"
```

### Validity Period

Default: 365 days. Specify custom:

```bash
./build/muti-metroo cert ca --days 730       # 2 years
./build/muti-metroo cert agent --days 90     # 90 days
```

### Key Size

- CA: 4096-bit RSA (recommended)
- Agent/Client: 2048-bit RSA or P-256 ECDSA

## Certificate Rotation

### Planned Rotation

1. Generate new certificates before expiration
2. Deploy new certificates alongside old ones
3. Update configuration to use new certificates
4. Restart agents
5. Remove old certificates

### Emergency Rotation

If CA is compromised:

1. Generate new CA
2. Generate new certificates for all agents
3. Deploy and restart all agents simultaneously
4. Revoke trust in old CA

## Monitoring Expiration

### CLI Check

```bash
./build/muti-metroo cert info ./certs/agent.crt
```

Output includes expiration date.

### OpenSSL Check

```bash
openssl x509 -enddate -noout -in ./certs/agent.crt
```

### Automated Monitoring

Add to your monitoring:

```bash
#!/bin/bash
# Check if cert expires in next 30 days
openssl x509 -checkend 2592000 -noout -in ./certs/agent.crt
if [ $? -eq 1 ]; then
  echo "Certificate expires soon!"
fi
```

## Troubleshooting

### Certificate Not Trusted

```
Error: x509: certificate signed by unknown authority
```

- Verify CA certificate is correct
- Check CA was used to sign agent certificate

### Certificate Expired

```
Error: x509: certificate has expired
```

- Generate new certificate
- Check system time is correct

### Name Mismatch

```
Error: x509: certificate is valid for agent1.example.com, not agent2.example.com
```

- Use correct hostname/IP
- Add SANs when generating certificate

### Private Key Mismatch

```
Error: tls: private key does not match public key
```

- Verify key matches certificate:
  ```bash
  openssl x509 -noout -modulus -in agent.crt | openssl md5
  openssl rsa -noout -modulus -in agent.key | openssl md5
  # Must match
  ```

## Best Practices

1. **Protect CA private key**: Use hardware security module or secure vault
2. **Use short validity**: 90-365 days for agent certs
3. **Use SANs**: Include all hostnames and IPs
4. **Enable mTLS**: Especially in production
5. **Automate rotation**: Before certificates expire
6. **Monitor expiration**: Alert before expiry

## Examples

### Development

```yaml
listeners:
  - transport: quic
    tls:
      cert: "./dev-certs/agent.crt"
      key: "./dev-certs/agent.key"
      # No client_ca - accept any peer
```

### Production with mTLS

```yaml
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

### Kubernetes

```yaml
listeners:
  - transport: quic
    tls:
      cert_pem: "${TLS_CRT}"      # From Secret
      key_pem: "${TLS_KEY}"       # From Secret
      client_ca_pem: "${CA_CRT}"  # From ConfigMap
```

## Related

- [CLI: cert](../cli/cert) - Certificate CLI commands
- [Security: TLS/mTLS](../security/tls-mtls) - Security considerations
- [Deployment](../deployment/scenarios) - Production deployment
