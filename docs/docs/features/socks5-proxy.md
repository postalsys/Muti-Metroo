---
title: SOCKS5 Proxy
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting SOCKS5" style={{maxWidth: '180px'}} />
</div>

# SOCKS5 Proxy

Muti Metroo provides SOCKS5 proxy ingress for client connections.

## Configuration

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$..."  # bcrypt hash
  max_connections: 1000
```

## Authentication

Supports two methods:

### No Authentication (Method 0x00)

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
```

### Username/Password (Method 0x02)

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
```

Generate password hash:

```bash
htpasswd -bnBC 10 "" password | tr -d ':\n'
```

## Usage Examples

### cURL

```bash
# No auth
curl -x socks5://localhost:1080 https://example.com

# With auth
curl -x socks5://user1:password@localhost:1080 https://example.com
```

### SSH

```bash
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@remote-host
```

### Firefox

1. Preferences -> Network Settings -> Manual proxy configuration
2. SOCKS Host: localhost, Port: 1080
3. SOCKS v5
4. Username/password if auth enabled

## Metrics

- `muti_metroo_socks5_connections_active`: Active connections
- `muti_metroo_socks5_connections_total`: Total connections
- `muti_metroo_socks5_auth_failures_total`: Auth failures
- `muti_metroo_socks5_connect_latency_seconds`: Connect latency

## Related

- [Configuration - SOCKS5](../configuration/socks5) - Full configuration reference
- [Security - Authentication](../security/authentication) - Password security
- [Concepts - Agent Roles](../concepts/agent-roles) - Understanding ingress role
- [Troubleshooting - Common Issues](../troubleshooting/common-issues) - SOCKS5 troubleshooting
