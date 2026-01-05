---
title: Best Practices
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole recommending best practices" style={{maxWidth: '180px'}} />
</div>

# Security Best Practices

Production hardening guide for Muti Metroo deployments.

## Certificate Management

### Protect CA Private Key

The CA private key is the most critical secret:

```bash
# Secure storage options:
# 1. Hardware Security Module (HSM)
# 2. Cloud KMS (AWS KMS, GCP KMS, Azure Key Vault)
# 3. HashiCorp Vault
# 4. Encrypted disk with strong passphrase

# Never:
# - Store in version control
# - Leave on shared systems
# - Transmit over unencrypted channels
```

### Short Certificate Validity

```bash
# CA: 1-3 years
muti-metroo cert ca --days 365

# Agent certificates: 90 days
muti-metroo cert agent --days 90
```

### Automate Certificate Rotation

```bash
#!/bin/bash
# rotate-cert.sh
CERT_DIR="/etc/muti-metroo/certs"
CA_DIR="/etc/muti-metroo/ca"  # CA stored separately
DAYS_LEFT=$(( ($(date -d "$(openssl x509 -enddate -noout -in $CERT_DIR/agent.crt | cut -d= -f2)" +%s) - $(date +%s)) / 86400 ))

if [ $DAYS_LEFT -lt 30 ]; then
    muti-metroo cert agent --cn "$(hostname)" \
      --ca "$CA_DIR/ca.crt" \
      --ca-key "$CA_DIR/ca.key" \
      -o "$CERT_DIR"
    systemctl restart muti-metroo
fi
```

### Monitor Certificate Expiration

Set up certificate expiration monitoring using external tools or scripts:

```bash
#!/bin/bash
# check-cert-expiry.sh
CERT="/etc/muti-metroo/certs/agent.crt"
DAYS_LEFT=$(( ($(date -d "$(openssl x509 -enddate -noout -in $CERT | cut -d= -f2)" +%s) - $(date +%s)) / 86400 ))

if [ $DAYS_LEFT -lt 14 ]; then
    echo "WARNING: Certificate expires in $DAYS_LEFT days"
    exit 1
fi
```

## Authentication Hardening

### Strong Passwords

```python
# Generate strong random password
import secrets
import string
alphabet = string.ascii_letters + string.digits + string.punctuation
password = ''.join(secrets.choice(alphabet) for _ in range(24))
print(password)
```

### High bcrypt Cost

```bash
# Use cost factor 12+ for production
htpasswd -bnBC 12 "" "$password" | tr -d ':\n'
```

### Enable All Authentication

```yaml
# Always enable auth when network-accessible
socks5:
  address: "0.0.0.0:1080"    # Network accessible
  auth:
    enabled: true             # Must be authenticated

# Shell should always require password
shell:
  enabled: true
  password_hash: "$2a$12$..."   # Strong hash
  whitelist:                    # Minimal commands
    - whoami
    - hostname
```

## Network Security

### Bind to Appropriate Interfaces

```yaml
# Internal only - bind to private interface
listeners:
  - address: "10.0.0.5:4433"

# SOCKS5 for local use only
socks5:
  address: "127.0.0.1:1080"

# API for monitoring
http:
  address: "127.0.0.1:8080"
```

### Use Firewall Rules

```bash
# Allow only necessary ports
iptables -A INPUT -p udp --dport 4433 -j ACCEPT  # QUIC
iptables -A INPUT -p tcp --dport 1080 -s 127.0.0.1 -j ACCEPT  # SOCKS5 local
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/8 -j ACCEPT  # API internal
```

### Enable mTLS

```yaml
# Always use mTLS in production
tls:
  ca: "/etc/muti-metroo/certs/ca.crt"      # CA for verifying client certs
  cert: "/etc/muti-metroo/certs/agent.crt"
  key: "/etc/muti-metroo/certs/agent.key"
  mtls: true                               # Require valid client certificates

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    # Uses global TLS settings with mTLS enabled
```

## Access Control

### Restrict Exit Routes

```yaml
# Principle of least privilege
exit:
  routes:
    - "10.0.1.0/24"    # Only needed subnet
    # NOT 0.0.0.0/0    # Never allow everything
```

### Minimal Shell Whitelist

```yaml
shell:
  whitelist:
    - whoami          # Safe commands only
    - hostname
    - uptime
    # NOT: sh, bash, python, cat, rm, etc.
```

### Restrict File Paths

```yaml
file_transfer:
  allowed_paths:
    - /tmp/transfers  # Dedicated directory
  max_file_size: 10485760  # 10 MB limit
```

## Disable Unused Features

```yaml
# Only enable what you need
shell:
  enabled: false      # Disable if not needed

file_transfer:
  enabled: false      # Disable if not needed

# If you don't need exit
exit:
  enabled: false
```

## Secure Secrets

### Use Environment Variables

```yaml
socks5:
  auth:
    users:
      - username: "${SOCKS5_USER}"
        password_hash: "${SOCKS5_PASSWORD_HASH}"

shell:
  password_hash: "${SHELL_PASSWORD_HASH}"
```

### Secure Secret Storage

```bash
# Use secret management tools:
# - HashiCorp Vault
# - AWS Secrets Manager
# - Kubernetes Secrets
# - Docker Secrets

# Never:
# - Commit secrets to git
# - Log secrets
# - Store in plain text files
```

## Monitoring and Logging

### Enable Structured Logging

```yaml
agent:
  log_level: "info"
  log_format: "json"    # For log aggregation
```

### Retain Audit Logs

```bash
# Keep logs for compliance/forensics
journalctl -u muti-metroo --since "30 days ago" > /archive/muti-metroo-$(date +%Y%m%d).log
```

## System Hardening

### Run as Non-Root

```bash
# Create dedicated user
useradd -r -s /sbin/nologin muti-metroo

# Set ownership
chown -R muti-metroo:muti-metroo /var/lib/muti-metroo
```

### Limit Capabilities

```ini
# systemd unit
[Service]
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
```

### File Permissions

```bash
chmod 700 /var/lib/muti-metroo          # Data dir
chmod 640 /etc/muti-metroo/config.yaml  # Config
chmod 600 /etc/muti-metroo/certs/*.key  # Private keys
chmod 644 /etc/muti-metroo/certs/*.crt  # Certificates
```

## Incident Response

### Prepare for Compromise

1. **Have backups** of configuration and certificates
2. **Document** certificate rotation procedure
3. **Know how to** revoke certificates
4. **Have contacts** for security team

### If CA is Compromised

1. Generate new CA immediately
2. Generate new certificates for all agents
3. Deploy new certificates to all agents
4. Update all configurations
5. Restart all agents
6. Investigate how compromise occurred

### If Agent is Compromised

1. Disconnect agent from mesh
2. Revoke agent's certificate
3. Investigate compromised host
4. Re-image or clean host
5. Generate new identity and certificates
6. Reconnect to mesh

## Security Checklist

### Pre-Deployment

- [ ] CA key stored securely (not in repo)
- [ ] All certificates have appropriate SANs
- [ ] mTLS enabled
- [ ] SOCKS5 authentication enabled
- [ ] Shell disabled or whitelist-restricted
- [ ] File transfer disabled or path-restricted
- [ ] Exit routes minimized

### Post-Deployment

- [ ] Firewall rules configured
- [ ] Monitoring and alerting set up
- [ ] Log aggregation configured
- [ ] Certificate expiration monitoring
- [ ] Regular security reviews scheduled
- [ ] Incident response plan documented

### Regular Maintenance

- [ ] Rotate certificates before expiration
- [ ] Review access controls quarterly
- [ ] Update to latest version
- [ ] Review security logs
- [ ] Test failover procedures

## Next Steps

- [TLS/mTLS](tls-mtls) - Certificate security
- [Authentication](authentication) - Password security
- [Access Control](access-control) - Restrict access
- [Troubleshooting](../troubleshooting/common-issues) - Security issues
