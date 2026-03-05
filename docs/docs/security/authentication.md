---
title: Authentication
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole checking authentication" style={{maxWidth: '180px'}} />
</div>

# Authentication

Control who can use your mesh. Require passwords for SOCKS5 proxy access, shell commands, and file transfers. Without valid credentials, requests are rejected.

## Where Authentication Applies

| Component | Authentication Method | What It Protects |
|-----------|----------------------|------------------|
| SOCKS5 proxy | Username + password | Who can tunnel traffic |
| Remote shell | Password | Who can run commands |
| File transfer | Password | Who can upload/download files |
| Peer connections | TLS certificates (mTLS) | Which agents can join the mesh |
| HTTP API | Bearer token (built-in) | Monitoring and management access |

## Password Hashing

All passwords are stored as bcrypt hashes - never as plaintext. Generate hashes using the built-in CLI:

```bash
# Interactive (recommended - password hidden)
muti-metroo hash

# With custom cost factor for production
muti-metroo hash --cost 12
```

See [hash command](/cli/hash) for full documentation.

### Cost Factor

The cost factor determines hash computation time:

| Cost | Time (approx) | Recommendation |
|------|---------------|----------------|
| 10 | ~100ms | Development |
| 12 | ~400ms | Production |
| 14 | ~1.5s | High security |

Higher cost = slower brute force attacks, but also slower authentication.

## SOCKS5 Authentication

Protect your proxy with username/password authentication.

:::tip Configuration
See [SOCKS5 Configuration](/configuration/socks5) for auth setup including multiple users.
:::

### Client Usage

```bash
# curl
curl -x socks5://user1:password@localhost:1080 https://example.com

# ssh via netcat
ssh -o ProxyCommand='nc -x localhost:1080 -P user1 %h %p' user@host

# Firefox: Enter credentials in Network Settings
```

## Shell Authentication

Protect remote command execution with password authentication.

:::tip Configuration
See [Remote Shell Configuration](/configuration/shell) for password setup and command whitelist.
:::

### Client Usage

```bash
# Simple command
muti-metroo shell -p mypassword agent123 whoami

# Interactive shell
muti-metroo shell --tty -p mypassword agent123 bash
```

## File Transfer Authentication

Protect file uploads and downloads with password authentication.

:::tip Configuration
See [File Transfer Configuration](/configuration/file-transfer) for password setup and path restrictions.
:::

### Client Usage

```bash
# Upload
muti-metroo upload -p mypassword agent123 ./local.txt /tmp/remote.txt

# Download
muti-metroo download -p mypassword agent123 /tmp/remote.txt ./local.txt
```

## HTTP API Security

The HTTP API supports built-in bearer token authentication:

```yaml
http:
  address: ":8080"
  token_hash: "$2a$10$..."  # bcrypt hash of your bearer token
```

When `token_hash` is set, all non-health endpoints require authentication via:
- `Authorization: Bearer <token>` header, or
- `?token=<token>` query parameter

Health endpoints (`/health`, `/healthz`, `/ready`) are always exempt.

Generate the hash with `muti-metroo hash`.

### Additional Security Measures

For defense in depth, you can also:

**Bind to localhost** (only local access):

```yaml
http:
  address: "127.0.0.1:8080"
```

**Firewall rules** (restrict to management network):

```bash
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

## Environment Variables

Never hardcode password hashes in config files. Use environment variables:

```yaml
socks5:
  auth:
    users:
      - username: "${SOCKS5_USER}"
        password_hash: "${SOCKS5_PASSWORD_HASH}"

shell:
  password_hash: "${SHELL_PASSWORD_HASH}"
```

See [Environment Variables](/configuration/environment-variables) for details.

## Defense in Depth

Layer multiple security mechanisms:

1. **Bind to localhost** - Restrict network access
2. **Require authentication** - Verify identity
3. **Use TLS/mTLS** - Encrypt and authenticate transport
4. **Firewall rules** - Network-level restrictions

Example: SOCKS5 proxy bound to localhost AND requiring authentication provides two layers of protection.

## Troubleshooting

### Invalid Password Hash

```
Error: invalid bcrypt hash
```

**Causes:**
- Hash doesn't start with `$2a$` or `$2b$`
- Hash was corrupted or truncated
- Extra whitespace in the hash

**Solution:** Regenerate using `muti-metroo hash`

### Authentication Failed

```
Error: authentication failed
```

**Causes:**
- Wrong password
- Wrong username (case-sensitive)
- Hash generated from different password

**Solution:** Verify credentials and regenerate hash if needed

### User Not Found (SOCKS5)

```
Error: user not found
```

**Causes:**
- Username not in config
- Username typo (case-sensitive)

**Solution:** Check `socks5.auth.users` in configuration

## Next Steps

- [Access Control](/security/access-control) - Route and command restrictions
- [TLS/mTLS](/security/tls-mtls) - Certificate-based authentication
- [Best Practices](/security/best-practices) - Production hardening
