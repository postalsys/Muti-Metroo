---
title: ICMP Relay
sidebar_position: 7
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-surfacing.png" alt="Mole with ICMP" style={{maxWidth: '180px'}} />
</div>

# ICMP Relay

Ping remote hosts through your mesh network. Test connectivity, measure latency, and diagnose network issues - all through your encrypted tunnel.

```bash
# Ping through the mesh
muti-metroo ping 8.8.8.8

# Continuous ping
muti-metroo ping -c 0 192.168.1.1

# IPv6 ping
muti-metroo ping 2001:4860:4860::8888
```

## How It Works

ICMP echo requests flow through your mesh just like TCP and UDP - encrypted end-to-end from ingress to exit.

```mermaid
flowchart LR
    A[CLI Client] -->|ICMP_OPEN| B[Ingress Agent]
    B -->|Encrypted Mesh| C[Transit Agents]
    C -->|Encrypted Mesh| D[Exit Agent]
    D -->|ICMP Echo| E[Destination]
```

All ICMP payloads are encrypted end-to-end between ingress and exit using ChaCha20-Poly1305. Transit nodes cannot decrypt the traffic.

## Requirements

ICMP relay requires:

1. **Exit agent** with ICMP enabled (enabled by default)
2. A route from ingress to exit
3. **Linux only**: `ping_group_range` sysctl configured (see below)

## Configuration

### Exit Node

ICMP is enabled by default on exit nodes:

```yaml
icmp:
  enabled: true
  max_sessions: 100
  idle_timeout: 60s
  echo_timeout: 5s
```

See [Configuration - ICMP](/configuration/icmp) for full reference.

### Linux System Requirements

On Linux, unprivileged ICMP sockets require kernel configuration:

```bash
# Check current setting
sysctl net.ipv4.ping_group_range

# Enable for all groups (run as root)
sudo sysctl -w net.ipv4.ping_group_range="0 65535"

# Make persistent
echo "net.ipv4.ping_group_range=0 65535" | sudo tee -a /etc/sysctl.conf
```

Without this setting, ICMP will fail with permission errors.

:::info No Root Required
Unlike traditional ping, Muti Metroo uses unprivileged ICMP sockets. Once the sysctl is configured, no root privileges are needed.
:::

## Usage

### Basic Ping

```bash
# Ping a host
muti-metroo ping 8.8.8.8

# Output:
# PING 8.8.8.8 via exit agent abc123def456
# 64 bytes from 8.8.8.8: seq=1 time=12.3ms
# 64 bytes from 8.8.8.8: seq=2 time=11.8ms
```

### Continuous Ping

```bash
# Ping indefinitely (Ctrl+C to stop)
muti-metroo ping -c 0 192.168.1.1
```

### IPv6 Ping

```bash
# Ping IPv6 addresses
muti-metroo ping 2001:4860:4860::8888
```

### Via Specific Agent

```bash
# Ping through a specific agent's API
muti-metroo ping -a 192.168.1.10:8080 8.8.8.8
```

## IPv4 and IPv6 Support

ICMP supports both IP versions automatically:

| Version | Protocol | Echo Request | Echo Reply |
|---------|----------|--------------|------------|
| IPv4 | ICMPv4 (1) | Type 8 | Type 0 |
| IPv6 | ICMPv6 (58) | Type 128 | Type 129 |

The agent detects the IP version from the destination address and uses the appropriate socket type.

## End-to-End Encryption

ICMP traffic is encrypted between ingress and exit:

1. Ingress generates ephemeral X25519 keypair
2. Exit generates ephemeral X25519 keypair
3. Both derive shared secret via ECDH
4. Each echo request/reply encrypted with ChaCha20-Poly1305

Transit nodes cannot decrypt ICMP payloads.

## Restricting Destinations

Limit which IPs can be pinged with CIDR whitelisting:

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    - "8.8.8.0/24"           # Google DNS
    - "1.1.1.0/24"           # Cloudflare DNS
    - "10.0.0.0/8"           # Internal network
    - "2001:4860::/32"       # Google IPv6
```

When `allowed_cidrs` is empty (default), all destinations are allowed.

## Troubleshooting

### Permission Denied (Linux)

```
Error: socket: operation not permitted
```

Configure the `ping_group_range` sysctl:

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 65535"
```

### ICMP Disabled

```
Error: ICMP not enabled
```

- Verify exit node has `icmp.enabled: true`
- Check that a route exists to the exit node
- Ensure exit node is connected to the mesh

### Destination Not Allowed

```
Error: destination not allowed
```

The exit node has `allowed_cidrs` configured and the destination IP is not in the whitelist.

### Session Timeout

Sessions expire after `idle_timeout` (default 60 seconds) of inactivity. Each ping request resets the timer.

## Security Considerations

1. **E2E encryption**: All ICMP data is encrypted through the mesh
2. **Session limits**: Use `max_sessions` to prevent resource exhaustion
3. **CIDR restrictions**: Use `allowed_cidrs` to limit pingable destinations
4. **No DNS resolution**: Only IP addresses are accepted (no domain names)

## Related

- [Configuration - ICMP](/configuration/icmp) - Full configuration reference
- [CLI - ping](/cli/ping) - CLI command reference
- [Features - UDP Relay](/features/udp-relay) - UDP tunneling
- [Configuration - Exit](/configuration/exit) - Exit node setup
