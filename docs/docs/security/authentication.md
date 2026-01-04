---
title: Authentication
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole checking authentication" style={{maxWidth: '180px'}} />
</div>

# Authentication

Muti Metroo supports multiple authentication mechanisms for different components.

## Authentication Overview

| Component | Mechanism | Purpose |
|-----------|-----------|---------|
| Peer connections | TLS/mTLS | Agent-to-agent authentication |
| SOCKS5 proxy | Username/password | Client authentication |
| Shell | bcrypt password | Command authorization |
| File transfer | bcrypt password | Transfer authorization |
| HTTP API | None (use firewall) | Monitoring endpoints |

## SOCKS5 Authentication

### Configuration

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
      - username: "user2"
        password_hash: "$2a$12$..."
```

### Generating Password Hashes

The recommended way to generate bcrypt password hashes is using the built-in CLI command:

```bash
# Interactive (recommended - password hidden)
muti-metroo hash

# Or provide password as argument
muti-metroo hash "yourpassword"

# With custom cost factor
muti-metroo hash --cost 12
```

See [Generating Password Hashes](/cli/hash) for detailed documentation.

#### Alternative Methods

Using htpasswd:

```bash
htpasswd -bnBC 10 "" yourpassword | tr -d ':\n'
```

Using Python:

```python
import bcrypt
print(bcrypt.hashpw(b"yourpassword", bcrypt.gensalt(10)).decode())
```

Using Node.js:

```javascript
const bcrypt = require('bcrypt');
console.log(bcrypt.hashSync('yourpassword', 10));
```

### Cost Factor

The cost factor (10, 12, etc.) determines hash computation time:

| Cost | Time (approx) | Recommendation |
|------|---------------|----------------|
| 10 | ~100ms | Development |
| 12 | ~400ms | Production |
| 14 | ~1.5s | High security |

Higher cost = slower brute force attacks, but also slower login.

### Multiple Users

```yaml
socks5:
  auth:
    enabled: true
    users:
      - username: "admin"
        password_hash: "$2a$12$..."
      - username: "readonly"
        password_hash: "$2a$12$..."
      - username: "automation"
        password_hash: "$2a$10$..."
```

### Client Usage

```bash
# curl
curl -x socks5://user1:password@localhost:1080 https://example.com

# ssh
ssh -o ProxyCommand='nc -x localhost:1080 -P user1 %h %p' user@host
```

## Shell Authentication

### Configuration

```yaml
shell:
  enabled: true
  whitelist:
    - bash
    - sh
  password_hash: "$2a$12$..."
  timeout: 60s
```

### Generating Shell Password Hash

Use the built-in CLI command (see [Generating Password Hashes](/cli/hash)):

```bash
muti-metroo hash --cost 12
```

### Using Shell with Authentication

CLI:

```bash
# Simple command (default streaming mode)
muti-metroo shell -p mypassword agent123 whoami

# Interactive shell (requires --tty)
muti-metroo shell --tty -p mypassword agent123 bash
```

## File Transfer Authentication

### Configuration

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$12$..."
  allowed_paths:
    - /tmp
    - /home/user/uploads
```

### Using File Transfer with Authentication

```bash
# Upload
muti-metroo upload -p mypassword agent123 ./local.txt /tmp/remote.txt

# Download
muti-metroo download -p mypassword agent123 /tmp/remote.txt ./local.txt
```

## HTTP API Authentication

The HTTP API does not have built-in authentication. Secure it using:

### Bind to Localhost

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"    # Only local access
```

### Firewall Rules

```bash
# Only allow from specific IP
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.100 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

### Reverse Proxy with Auth

nginx example:

```nginx
server {
    listen 443 ssl;
    server_name api.example.com;

    auth_basic "Restricted";
    auth_basic_user_file /etc/nginx/.htpasswd;

    location / {
        proxy_pass http://127.0.0.1:8080;
    }
}
```

## Environment Variables

Never hardcode passwords in config files:

```yaml
socks5:
  auth:
    users:
      - username: "${SOCKS5_USER}"
        password_hash: "${SOCKS5_PASSWORD_HASH}"

shell:
  password_hash: "${SHELL_PASSWORD_HASH}"

file_transfer:
  password_hash: "${FILE_TRANSFER_PASSWORD_HASH}"
```

## Best Practices

### Password Security

1. **Use strong passwords**: 16+ characters, random
2. **Use high cost factor**: 12+ for production
3. **Rotate passwords regularly**: Especially if exposed
4. **Never share passwords**: Per-user or per-system credentials

### Defense in Depth

Layer multiple security mechanisms:

```yaml
# Localhost binding + authentication
socks5:
  address: "127.0.0.1:1080"    # Only local
  auth:
    enabled: true              # And authenticated
```

## Troubleshooting

### Invalid Password Hash

```
Error: invalid bcrypt hash
```

- Verify hash starts with `$2a$` or `$2b$`
- Check hash was generated correctly
- Ensure no extra whitespace

### Authentication Failed

```
Error: authentication failed
```

- Verify password is correct
- Check password hash in config
- Enable debug logging

### User Not Found

```
Error: user not found
```

- Check username spelling
- Verify user is in config
- Username is case-sensitive

## Next Steps

- [Access Control](access-control) - Route and command restrictions
- [Best Practices](best-practices) - Production hardening
- [TLS/mTLS](tls-mtls) - Certificate authentication
