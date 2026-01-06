---
title: FAQ
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole reading FAQ" style={{maxWidth: '180px'}} />
</div>

# Frequently Asked Questions

## General

### What is Muti Metroo?

Muti Metroo is a userspace mesh networking agent that creates virtual TCP tunnels across different transport protocols. It enables multi-hop routing with SOCKS5 proxy ingress and CIDR-based exit routing.

### Do I need root to run Muti Metroo?

No. Muti Metroo runs entirely in userspace. However, you may need root for:
- Ports below 1024
- Installing as a system service
- Setting capabilities

### What platforms are supported?

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

### What Go version is required?

Go 1.23 or later for building from source.

## Networking

### What transport should I use?

| Use Case | Recommended |
|----------|-------------|
| Best performance | QUIC |
| Corporate firewall | HTTP/2 or WebSocket |
| Through HTTP proxy | WebSocket |
| Maximum compatibility | WebSocket |

See [Transports](/concepts/transports) for details.

### How many hops can I have?

Theoretically, up to 255 (max_hops). Practically, limit to:
- 8-12 for interactive use (SSH)
- 6-10 for streaming
- 4-6 for high-latency WAN

### Can I mix transport types?

Yes. Each peer connection can use a different transport:

```yaml
peers:
  - transport: quic
    address: "direct-peer:4433"
  - transport: ws
    address: "wss://proxy-peer:443/mesh"
```

### Does Muti Metroo support IPv6?

Yes. IPv6 is fully supported across the stack:

**Listeners** - Bind to IPv6 addresses:
```yaml
listeners:
  - transport: quic
    address: "[::]:4433"        # All IPv6 interfaces
    # or
    address: "[::1]:4433"       # IPv6 localhost only
```

**Peers** - Connect to IPv6 peers:
```yaml
peers:
  - transport: quic
    address: "[2001:db8::1]:4433"
```

**Exit routes** - Advertise IPv6 CIDRs:
```yaml
exit:
  routes:
    - "0.0.0.0/0"    # IPv4 default
    - "::/0"         # IPv6 default
    - "2001:db8::/32"
```

**DNS servers** - Use IPv6 resolvers:
```yaml
exit:
  dns:
    servers:
      - "[2001:4860:4860::8888]:53"  # Google IPv6 DNS
```

**SOCKS5** - Bind to IPv6 and proxy IPv6 destinations:
```yaml
socks5:
  address: "[::1]:1080"    # IPv6 localhost
```

**Limitations:**
- DNS resolution prefers IPv4 when both A and AAAA records exist
- Node info advertisements only include IPv4 addresses

## Security

### How is traffic encrypted?

All peer connections use TLS 1.3 with:
- AES-256-GCM or ChaCha20-Poly1305 encryption
- ECDHE key exchange
- Perfect forward secrecy

### Is mutual TLS (mTLS) required?

No, but it's recommended for production. Without mTLS, any client can connect if they can reach the port.

### Can I use Let's Encrypt certificates?

Yes, for listeners. However, you'll also need:
- A CA for signing client certificates (for mTLS)
- Agent certificates signed by your CA

### How do I rotate certificates?

1. Generate new certificates before old ones expire
2. Deploy new certificates
3. Restart agents
4. Remove old certificates

See [TLS Certificates](/configuration/tls-certificates) for details.

## SOCKS5

### What SOCKS5 features are supported?

- CONNECT command: Yes
- BIND: No
- UDP ASSOCIATE: No
- IPv4/IPv6: Yes
- Domain names: Yes
- No auth: Yes
- Username/password: Yes

### Can I use SOCKS5 with browsers?

Yes. Configure your browser's proxy settings:
- SOCKS Host: localhost (or agent address)
- Port: 1080 (or configured port)
- SOCKS version: 5

### How do I use SOCKS5 with SSH?

```bash
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@host
```

Or in `~/.ssh/config`:

```
Host myhost
  ProxyCommand nc -x localhost:1080 %h %p
```

## Performance

### How much memory does Muti Metroo use?

Base memory plus:
- 256 KB per stream per hop (default buffer)

Example: 1000 streams x 3 hops x 256 KB = 768 MB

### How fast is Muti Metroo?

Throughput depends on:
- Network speed between hops
- Number of hops
- Transport type (QUIC is fastest)
- Buffer size

Latency overhead:
- LAN: 1-5ms per hop
- WAN: 50-200ms per hop

### Can I run multiple agents on one machine?

Yes. Use different:
- Data directories
- Port numbers
- Agent IDs

## Configuration

### Can I reload configuration without restart?

Currently, no. Restart the agent to apply configuration changes.

### How do I use environment variables?

```yaml
agent:
  log_level: "${LOG_LEVEL:-info}"

socks5:
  auth:
    users:
      - username: "${USER}"
        password_hash: "${PASS_HASH}"
```

### Where should I put the configuration file?

- Development: `./config.yaml`
- Production: `/etc/muti-metroo/config.yaml`
- Docker: `/app/config.yaml`

## Deployment

### How do I run Muti Metroo as a service?

```bash
# Linux
sudo ./muti-metroo service install -c /etc/muti-metroo/config.yaml
sudo systemctl enable --now muti-metroo

# Windows (as Administrator)
muti-metroo.exe service install -c C:\config\config.yaml
```

### Can I run Muti Metroo in Docker?

Yes. See [Docker Deployment](/deployment/docker).

### Can I run Muti Metroo in Kubernetes?

Yes. See [Kubernetes Deployment](/deployment/kubernetes).

## Troubleshooting

### How do I enable debug logging?

```yaml
agent:
  log_level: "debug"
```

Or at runtime:

```bash
./muti-metroo run -c config.yaml --log-level debug
```

### How do I check if agents are connected?

```bash
curl http://localhost:8080/healthz | jq '.peers'
```

### How do I check routes?

```bash
curl http://localhost:8080/healthz | jq '.routes'
```

### Where are the logs?

- Foreground: stderr
- systemd: `journalctl -u muti-metroo`
- Docker: `docker logs <container>`

### How do I get help?

1. Check this documentation
2. Enable debug logging and review logs
3. Search existing issues
4. Open a new issue with:
   - Configuration (redacted)
   - Logs
   - Steps to reproduce

## Features

### Can I execute commands on remote agents?

Yes, using remote shell:

```bash
# Simple command (default normal mode)
muti-metroo shell agent-id whoami

# Interactive shell (requires --tty)
muti-metroo shell --tty agent-id bash
```

Shell must be enabled and configured. See [Remote Shell](/features/shell).

### Can I transfer files?

Yes:

```bash
# Upload
muti-metroo upload agent-id ./local.txt /remote/path.txt

# Download
muti-metroo download agent-id /remote/path.txt ./local.txt
```

File transfer must be enabled. See [File Transfer](/features/file-transfer).

### Is there a web interface?

Yes. Access the dashboard at:

```
http://localhost:8080/ui/
```

See [Web Dashboard](/features/web-dashboard).

## Next Steps

- [Common Issues](/troubleshooting/common-issues)
- [Connectivity Troubleshooting](/troubleshooting/connectivity)
- [Performance Troubleshooting](/troubleshooting/performance)
