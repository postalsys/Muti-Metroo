# SOCKS5 Proxy

Muti Metroo provides SOCKS5 proxy ingress for client connections, supporting both TCP (CONNECT) and UDP (UDP ASSOCIATE) per RFC 1928.

## Configuration

```yaml
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password_hash: "$2a$10$..."
  max_connections: 1000
```

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
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "operator1"
        password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
      - username: "operator2"
        password_hash: "$2a$10$..."
```

Generate password hash:

```bash
muti-metroo hash --cost 12
Enter password:
Confirm password:
$2a$12$...
```

## Usage Examples

### cURL

```bash
# No auth
curl -x socks5://localhost:1080 https://example.com

# With auth
curl -x socks5://user1:password@localhost:1080 https://example.com

# With hostname resolution at proxy (socks5h)
curl -x socks5h://localhost:1080 https://example.com
```

### SSH

```bash
# Using netcat
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@remote-host

# Using ncat
ssh -o ProxyCommand='ncat --proxy localhost:1080 --proxy-type socks5 %h %p' user@remote-host
```

### Firefox

1. Open Preferences -> Network Settings -> Manual proxy configuration
2. SOCKS Host: `localhost`, Port: `1080`
3. Select SOCKS v5
4. Enter username/password if authentication is enabled

### Git

```bash
# Clone via SOCKS5
git -c http.proxy=socks5://localhost:1080 clone https://github.com/repo/name

# Configure globally
git config --global http.proxy socks5://localhost:1080
```

### Python

```python
import socks
import socket

socks.set_default_proxy(socks.SOCKS5, "localhost", 1080)
socket.socket = socks.socksocket
```

### proxychains

Configure `/etc/proxychains.conf`:

```
[ProxyList]
socks5 127.0.0.1 1080
```

Use any application through the proxy:

```bash
proxychains4 curl https://example.com
proxychains4 nmap -sT target
```

## Bind Address Options

| Address | Access |
|---------|--------|
| `127.0.0.1:1080` | Local only (most secure) |
| `0.0.0.0:1080` | All network interfaces |
| `192.168.1.10:1080` | Specific interface |

## Connection Limits

Limit concurrent connections to prevent resource exhaustion:

```yaml
socks5:
  max_connections: 1000    # Maximum concurrent connections
```

## UDP Support

SOCKS5 UDP ASSOCIATE enables UDP traffic tunneling (DNS, NTP) through the mesh.

On the exit node, configure UDP relay:

```yaml
udp:
  enabled: true
  max_associations: 1000
  idle_timeout: 5m
```

Test with proxychains:

```bash
proxychains4 dig @8.8.8.8 example.com
```

## Security Considerations

1. **Bind to localhost** when possible to prevent unauthorized access
2. **Enable authentication** when binding to network interfaces
3. **Use strong passwords** with bcrypt hashing
4. **Monitor connections** via HTTP API health endpoints
5. **Limit max_connections** to prevent DoS

## Alternative: TUN Interface

For transparent proxying without application configuration, see [TUN Interface (Mutiauk)](10-tun-interface.md) (Linux only). The TUN interface captures all IP traffic, enabling any application to use the mesh without SOCKS5 support.
