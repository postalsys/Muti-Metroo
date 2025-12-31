---
title: Access Control
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole controlling access" style={{maxWidth: '180px'}} />
</div>

# Access Control

Control what destinations, commands, and paths are accessible.

## Route-Based Access Control

Exit nodes only allow connections to advertised routes:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"           # Allow internal network
    - "192.168.1.0/24"       # Allow specific subnet
    # NOT 0.0.0.0/0         # Internet access denied
```

### How It Works

1. Client connects to destination via SOCKS5
2. Ingress agent routes to exit agent
3. Exit agent checks if destination matches advertised routes
4. If no match: `STREAM_OPEN_ERR` returned
5. If match: Connection established

### Example Configurations

**Internal Network Only:**

```yaml
exit:
  routes:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

**Specific Services:**

```yaml
exit:
  routes:
    - "10.0.1.10/32"    # Database server
    - "10.0.1.20/32"    # API server
    - "10.0.1.30/32"    # File server
```

**Split Internet:**

```yaml
# Exit 1: Internal only
exit:
  routes:
    - "10.0.0.0/8"

# Exit 2: Internet only (exclude internal)
exit:
  routes:
    - "0.0.0.0/0"
    # Routes propagate with metrics, internal goes to Exit 1
```

## RPC Command Whitelist

Only whitelisted commands can be executed:

```yaml
rpc:
  enabled: true
  whitelist:
    - whoami
    - hostname
    - ip
    - df
    - uptime
  password_hash: "$2a$12$..."
```

### Whitelist Options

**Empty list (default)**: No commands allowed:

```yaml
rpc:
  whitelist: []
```

**Specific commands**:

```yaml
rpc:
  whitelist:
    - whoami
    - hostname
```

**All commands** (DANGEROUS - testing only):

```yaml
rpc:
  whitelist:
    - "*"
```

### What Can Be Whitelisted

- Command base names only (`whoami`, not `/usr/bin/whoami`)
- Each command must be explicitly listed
- Arguments are not restricted (use with caution)

### Example Secure Whitelist

```yaml
rpc:
  whitelist:
    # System info
    - whoami
    - hostname
    - uptime
    - uname

    # Network diagnostics
    - ip
    - ping
    - traceroute
    - dig
    - nslookup

    # Disk info
    - df
    - du

    # Process info
    - ps
    - top
```

### What NOT to Whitelist

Avoid commands that can:
- Modify system (`rm`, `mv`, `cp`, `chmod`)
- Execute arbitrary code (`sh`, `bash`, `python`, `eval`)
- Access sensitive data (`cat`, `less`, `grep`)
- Elevate privileges (`sudo`, `su`)

## File Transfer Path Restrictions

Limit file transfer to specific directories:

```yaml
file_transfer:
  enabled: true
  allowed_paths:
    - /tmp
    - /home/app/uploads
    - /var/log/app
  password_hash: "$2a$12$..."
```

### How It Works

1. Upload/download request received
2. Path checked against allowed_paths
3. Path must start with one of allowed paths
4. If no match: Request denied
5. If match: Transfer proceeds

### Path Checking

```yaml
allowed_paths:
  - /tmp
  - /home/user/uploads

# Allowed:
# /tmp/file.txt         - matches /tmp
# /tmp/subdir/file.txt  - matches /tmp
# /home/user/uploads/x  - matches /home/user/uploads

# Denied:
# /etc/passwd           - no match
# /home/user/secret     - no match
# /tmp/../etc/passwd    - normalized, doesn't match
```

### Maximum File Size

```yaml
file_transfer:
  max_file_size: 104857600    # 100 MB limit
  # 0 = unlimited
```

## Network-Level Access Control

### Firewall Rules

Restrict who can connect to the agent:

```bash
# Linux (iptables)
# Only allow QUIC from specific IP
iptables -A INPUT -p udp --dport 4433 -s 10.0.0.100 -j ACCEPT
iptables -A INPUT -p udp --dport 4433 -j DROP

# Only allow SOCKS5 from localhost
iptables -A INPUT -p tcp --dport 1080 -s 127.0.0.1 -j ACCEPT
iptables -A INPUT -p tcp --dport 1080 -j DROP
```

### Bind Address

Limit listening interfaces:

```yaml
# Localhost only
socks5:
  address: "127.0.0.1:1080"

# Specific interface
listeners:
  - address: "192.168.1.10:4433"    # Not 0.0.0.0
```

### mTLS Certificate-Based ACL

Only specific clients can connect:

```yaml
listeners:
  - tls:
      client_ca: "./certs/ca.crt"    # Only certs signed by this CA
```

Combined with separate CAs for different access levels.

## Layered Security Example

Combine multiple controls:

```yaml
# Layer 1: Network binding
socks5:
  address: "10.0.0.5:1080"    # Internal interface only

# Layer 2: Authentication
socks5:
  auth:
    enabled: true
    users:
      - username: "internal-user"
        password_hash: "$2a$12$..."

# Layer 3: TLS mutual auth
listeners:
  - tls:
      client_ca: "./certs/internal-ca.crt"

# Layer 4: Route restriction
exit:
  routes:
    - "10.0.0.0/8"    # Internal only

# Layer 5: RPC restriction
rpc:
  enabled: true
  whitelist:
    - whoami
    - hostname
  password_hash: "$2a$12$..."

# Layer 6: File path restriction
file_transfer:
  enabled: true
  allowed_paths:
    - /tmp/transfers
  max_file_size: 10485760
  password_hash: "$2a$12$..."
```

## Monitoring Access

### Metrics

```bash
# SOCKS5 auth failures
curl http://localhost:8080/metrics | grep socks5_auth_failures

# RPC rejections
curl http://localhost:8080/metrics | grep rpc.*rejected

# Exit connection errors
curl http://localhost:8080/metrics | grep exit_errors
```

### Logging

Enable debug logging for access issues:

```yaml
agent:
  log_level: "debug"
```

Look for:
- `SOCKS5 auth failed`
- `RPC command rejected`
- `route not found`
- `file transfer denied`

## Best Practices

1. **Principle of least privilege**: Only allow what's needed
2. **Defense in depth**: Multiple layers of control
3. **Monitor and alert**: Track access failures
4. **Regular review**: Audit allowed routes/commands/paths
5. **Document access**: Know who has access to what

## Next Steps

- [Best Practices](best-practices) - Production hardening
- [Authentication](authentication) - Password security
- [Troubleshooting](../troubleshooting/common-issues) - Debug access issues
