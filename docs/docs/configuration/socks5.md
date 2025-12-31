---
title: SOCKS5
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring SOCKS5" style={{maxWidth: '180px'}} />
</div>

# SOCKS5 Configuration

The SOCKS5 section configures the ingress proxy server.

## Configuration

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
    users: []
  max_connections: 1000
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | false | Enable SOCKS5 server |
| `address` | string | "127.0.0.1:1080" | Bind address |
| `auth.enabled` | bool | false | Require authentication |
| `auth.users` | array | [] | User credentials |
| `max_connections` | int | 1000 | Maximum concurrent connections |

## Basic Configuration

### Local Access Only

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
```

### Network Access

```yaml
socks5:
  enabled: true
  address: "0.0.0.0:1080"    # Accept from any interface
```

:::warning
Exposing SOCKS5 on 0.0.0.0 without authentication allows anyone to use your proxy.
:::

## Authentication

### No Authentication

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
```

### Username/Password Authentication

```yaml
socks5:
  enabled: true
  address: "0.0.0.0:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
      - username: "user2"
        password_hash: "$2a$10$..."
```

### Generating Password Hash

Use the built-in CLI command (recommended):

```bash
# Interactive (recommended - password hidden)
muti-metroo hash

# Or provide password as argument
muti-metroo hash "yourpassword"

# With custom cost factor
muti-metroo hash --cost 12
```

See [CLI - hash](/cli/hash) for full documentation.

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

### Plaintext Passwords (Development Only)

```yaml
socks5:
  auth:
    users:
      - username: "dev"
        password: "devpass"    # NOT for production!
```

:::danger
Never use plaintext passwords in production. Always use bcrypt hashes.
:::

## Connection Limits

```yaml
socks5:
  max_connections: 1000       # Maximum concurrent SOCKS5 connections
```

When limit is reached:
- New connections are rejected
- Metric `socks5_connections_rejected_total` increments
- Existing connections continue working

## Client Configuration

### curl

```bash
# No authentication
curl -x socks5://localhost:1080 https://example.com

# With authentication
curl -x socks5://user1:password@localhost:1080 https://example.com

# SOCKS5h (resolve DNS through proxy)
curl -x socks5h://localhost:1080 https://example.com
```

### SSH

```bash
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@remote-host
```

Or in `~/.ssh/config`:

```
Host remote-host
  ProxyCommand nc -x localhost:1080 %h %p
  User myuser
```

### Firefox

1. Settings -> Network Settings
2. Manual proxy configuration
3. SOCKS Host: localhost, Port: 1080
4. SOCKS v5
5. If auth enabled, enter credentials when prompted

### Chrome

```bash
# Linux/macOS
google-chrome --proxy-server="socks5://localhost:1080"

# Windows
chrome.exe --proxy-server="socks5://localhost:1080"
```

### Git

```bash
git config --global http.proxy socks5://localhost:1080
```

## SOCKS5 Protocol Support

Muti Metroo supports:

| Feature | Supported |
|---------|-----------|
| CONNECT command | Yes |
| BIND command | No |
| UDP ASSOCIATE | No |
| No Auth (0x00) | Yes |
| Username/Password (0x02) | Yes |
| IPv4 addresses | Yes |
| IPv6 addresses | Yes |
| Domain names | Yes |

## Examples

### Development

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
```

### Team Shared Proxy

```yaml
socks5:
  enabled: true
  address: "0.0.0.0:1080"
  auth:
    enabled: true
    users:
      - username: "team"
        password_hash: "$2a$10$..."
  max_connections: 5000
```

### High-Security

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"    # Localhost only
  auth:
    enabled: true
    users:
      - username: "admin"
        password_hash: "$2a$12$..."    # Cost factor 12
  max_connections: 100
```

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_socks5_connections_active` | Gauge | Active connections |
| `muti_metroo_socks5_connections_total` | Counter | Total connections |
| `muti_metroo_socks5_auth_failures_total` | Counter | Auth failures |
| `muti_metroo_socks5_connect_latency_seconds` | Histogram | Connect latency |

## Troubleshooting

### Connection Refused

```bash
# Check SOCKS5 is enabled
grep -A5 "socks5:" config.yaml

# Check process is listening
lsof -i :1080
netstat -tlnp | grep 1080
```

### Authentication Failed

```bash
# Verify password hash
# Hash should start with $2a$ or $2b$

# Try with curl
curl -v -x socks5://user:pass@localhost:1080 https://example.com
```

### No Route to Host

- Check exit agent is connected
- Verify routes are advertised
- Check routing table: `curl http://localhost:8080/healthz`

See [Troubleshooting](../troubleshooting/common-issues) for more help.

## Related

- [Features: SOCKS5 Proxy](../features/socks5-proxy) - Detailed usage
- [Exit Configuration](exit) - Route configuration
- [Security](../security/authentication) - Authentication best practices
