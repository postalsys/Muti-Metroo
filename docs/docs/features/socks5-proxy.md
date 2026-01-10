---
title: SOCKS5 Proxy
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting SOCKS5" style={{maxWidth: '180px'}} />
</div>

# SOCKS5 Proxy

Route any TCP application through your mesh - curl, SSH, browsers, database clients, or any tool that supports SOCKS5. Point your application at the proxy and traffic flows through the mesh to its destination.

```bash
# These just work once your mesh is running
curl -x socks5://localhost:1080 https://internal.example.com
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@remote-host
```

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

### No Authentication

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: false
```

### Username/Password

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

Generate password hash using the built-in CLI:

```bash
muti-metroo hash --cost 12
```

See [CLI - hash](/cli/hash) for details.

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

1. Preferences → Network Settings → Manual proxy configuration
2. SOCKS Host: localhost, Port: 1080
3. SOCKS v5
4. Username/password if auth enabled

## UDP Support

SOCKS5 UDP ASSOCIATE is supported for tunneling UDP traffic (DNS, NTP) through the mesh. UDP relay requires an exit node with UDP enabled.

```yaml
# Exit node configuration
udp:
  enabled: true
```

See [Features - UDP Relay](/features/udp-relay) for details.

## Related

- [Features - UDP Relay](/features/udp-relay) - UDP tunneling through SOCKS5
- [Configuration - SOCKS5](/configuration/socks5) - Full configuration reference
- [Configuration - UDP](/configuration/udp) - UDP relay configuration
- [Security - Authentication](/security/authentication) - Password security
- [Concepts - Agent Roles](/concepts/agent-roles) - Understanding ingress role
- [Troubleshooting - Common Issues](/troubleshooting/common-issues) - SOCKS5 troubleshooting
