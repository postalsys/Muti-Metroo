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

# With hostname resolution at proxy (socks5h)
curl -x socks5h://localhost:1080 https://internal.corp.local
```

:::tip socks5 vs socks5h
Use `socks5h://` (note the `h`) when the destination hostname should be resolved by the exit agent rather than locally. This is essential for accessing internal DNS names that aren't resolvable from your machine.
:::

### SSH

```bash
# Using netcat (nc)
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@remote-host

# Using ncat (from nmap)
ssh -o ProxyCommand='ncat --proxy localhost:1080 --proxy-type socks5 %h %p' user@remote-host
```

### Firefox

1. Preferences → Network Settings → Manual proxy configuration
2. SOCKS Host: localhost, Port: 1080
3. SOCKS v5
4. Username/password if auth enabled

### Git

```bash
# Clone via SOCKS5
git -c http.proxy=socks5://localhost:1080 clone https://github.com/repo/name

# Configure globally
git config --global http.proxy socks5://localhost:1080

# Remove global config
git config --global --unset http.proxy
```

### Python

```python
import socks
import socket

# Configure default socket to use SOCKS5
socks.set_default_proxy(socks.SOCKS5, "localhost", 1080)
socket.socket = socks.socksocket

# Now all socket connections go through the proxy
import urllib.request
urllib.request.urlopen("https://example.com")
```

Requires [PySocks](https://pypi.org/project/PySocks/): `pip install pysocks`

### proxychains

Configure `/etc/proxychains.conf` or `~/.proxychains/proxychains.conf`:

```ini
[ProxyList]
socks5 127.0.0.1 1080
```

Run any application through the proxy:

```bash
proxychains4 curl https://example.com
proxychains4 nmap -sT -Pn 192.168.1.0/24
proxychains4 sqlmap -u "http://target/page?id=1"
```

## Bind Address Options

Control who can connect to the SOCKS5 proxy:

| Address | Access | Use Case |
|---------|--------|----------|
| `127.0.0.1:1080` | Local only | Most secure - only local applications |
| `0.0.0.0:1080` | All interfaces | Share proxy with other machines |
| `192.168.1.10:1080` | Specific interface | Limit to one network |

:::warning Network Binding
When binding to `0.0.0.0` or a network interface, always enable authentication to prevent unauthorized access.
:::

## Connection Limits

Prevent resource exhaustion with connection limits:

```yaml
socks5:
  max_connections: 1000    # Maximum concurrent connections (0 = unlimited)
```

## Security Considerations

1. **Bind to localhost** when possible to prevent unauthorized access
2. **Enable authentication** when binding to network interfaces
3. **Use strong passwords** with bcrypt hashing (cost 10+)
4. **Monitor connections** via the HTTP API health endpoints
5. **Set connection limits** to prevent resource exhaustion

## Transparent Proxying

For applications that don't support SOCKS5, use [Mutiauk](/mutiauk) (Linux only) to create a TUN interface that transparently routes traffic through the proxy. This enables tools like `nmap` to work without SOCKS configuration.

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
