---
title: UDP
sidebar_position: 8
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring UDP" style={{maxWidth: '180px'}} />
</div>

# UDP Configuration

The UDP section configures UDP relay for exit nodes, enabling SOCKS5 UDP ASSOCIATE support.

## Configuration

```yaml
udp:
  enabled: true
  allowed_ports:
    - "53"
    - "123"
  max_associations: 1000
  idle_timeout: 5m
  max_datagram_size: 1472
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | false | Enable UDP relay |
| `allowed_ports` | array | [] | Port whitelist |
| `max_associations` | int | 1000 | Maximum concurrent UDP associations |
| `idle_timeout` | duration | 5m | Association timeout after inactivity |
| `max_datagram_size` | int | 1472 | Maximum UDP payload size in bytes |

## Port Whitelist

The `allowed_ports` array controls which destination ports are permitted:

```yaml
udp:
  allowed_ports:
    - "53"     # DNS
    - "123"    # NTP
    - "5353"   # mDNS
```

### Special Values

| Value | Description |
|-------|-------------|
| `[]` | No ports allowed (effectively disables UDP) |
| `["*"]` | All ports allowed (use with caution) |
| `["53"]` | Only port 53 allowed |

### Common Ports

| Port | Protocol | Use Case |
|------|----------|----------|
| 53 | DNS | Domain name resolution |
| 123 | NTP | Time synchronization |
| 5353 | mDNS | Multicast DNS |
| 67-68 | DHCP | Dynamic IP assignment |

## Association Limits

Control resource usage with association limits:

```yaml
udp:
  max_associations: 1000    # Per exit node
  idle_timeout: 5m          # Close inactive associations
```

### max_associations

Maximum number of concurrent UDP associations. When the limit is reached, new UDP ASSOCIATE requests are rejected with an error.

Set to `0` for unlimited associations (not recommended for production).

### idle_timeout

Time after which inactive associations are closed. The timer resets on each datagram sent or received.

## Datagram Size

```yaml
udp:
  max_datagram_size: 1472
```

Maximum UDP payload size in bytes. The default (1472) is calculated as:

```
MTU (1500) - IP header (20) - UDP header (8) = 1472 bytes
```

Datagrams exceeding this size are rejected.

## Examples

### DNS Only

Minimal configuration for DNS relay:

```yaml
udp:
  enabled: true
  allowed_ports:
    - "53"
```

### DNS and NTP

Allow common time-sensitive protocols:

```yaml
udp:
  enabled: true
  allowed_ports:
    - "53"     # DNS
    - "123"    # NTP
  idle_timeout: 2m
```

### Testing (All Ports)

For testing environments only:

```yaml
udp:
  enabled: true
  allowed_ports:
    - "*"      # WARNING: All ports allowed
  max_associations: 100
  idle_timeout: 1m
```

:::warning
Never use `["*"]` in production. Always specify explicit ports.
:::

### High-Capacity Exit

For exit nodes handling many clients:

```yaml
udp:
  enabled: true
  allowed_ports:
    - "53"
    - "123"
  max_associations: 10000
  idle_timeout: 10m
  max_datagram_size: 1472
```

### Disabled (Default)

UDP relay is disabled by default:

```yaml
udp:
  enabled: false
```

Or simply omit the `udp` section entirely.

## Complete Exit Configuration

UDP relay works alongside TCP exit routing:

```yaml
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s

udp:
  enabled: true
  allowed_ports:
    - "53"
    - "123"
  max_associations: 1000
  idle_timeout: 5m
```

## Troubleshooting

### UDP ASSOCIATE Rejected

Check that:

1. `udp.enabled` is `true`
2. Exit node is connected to mesh
3. Route exists from ingress to exit

### Port Blocked

If a specific port is blocked:

```
Error: port 5000 not allowed
```

Add the port to `allowed_ports`:

```yaml
udp:
  allowed_ports:
    - "53"
    - "5000"   # Add required port
```

### Too Many Associations

If new associations are rejected:

```
Error: UDP association limit reached
```

Increase `max_associations` or reduce `idle_timeout` to free resources faster.

### Datagram Rejected

If datagrams are rejected for size:

```
Error: datagram too large
```

The payload exceeds `max_datagram_size`. Either:
- Reduce payload size in the application
- Increase `max_datagram_size` (not recommended above MTU)

## Security

1. **Explicit whitelist**: Always list specific ports
2. **Avoid wildcards**: Never use `["*"]` in production
3. **Limit associations**: Set reasonable `max_associations`
4. **Short timeouts**: Use shorter `idle_timeout` for high-traffic nodes

## Related

- [Features - UDP Relay](../features/udp-relay) - Feature overview
- [Configuration - Exit](./exit) - Exit node configuration
- [Configuration - SOCKS5](./socks5) - SOCKS5 ingress setup
