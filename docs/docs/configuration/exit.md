---
title: Exit
sidebar_position: 6
---

# Exit Configuration

The exit section configures the agent as an exit node that opens connections to external destinations.

## Configuration

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | false | Enable exit node |
| `routes` | array | [] | CIDR routes to advertise |
| `dns.servers` | array | [] | DNS servers for resolution |
| `dns.timeout` | duration | 5s | DNS query timeout |

## Routes

Routes define which destinations this exit node can reach:

```yaml
exit:
  routes:
    - "10.0.0.0/8"         # Private class A
    - "172.16.0.0/12"      # Private class B
    - "192.168.0.0/16"     # Private class C
    - "0.0.0.0/0"          # Default route (all traffic)
```

### Route Types

| Route | Description |
|-------|-------------|
| `10.0.0.0/8` | Internal network |
| `192.168.1.0/24` | Specific subnet |
| `1.2.3.4/32` | Single host |
| `0.0.0.0/0` | Default route (internet) |

### Route Selection

When multiple exit nodes advertise overlapping routes:

1. **Longest prefix wins**: `/32` beats `/24` beats `/0`
2. **Lower metric wins**: Closer exit preferred

Example:
- Exit A advertises `10.0.0.0/8` (metric 1)
- Exit B advertises `10.1.0.0/16` (metric 2)
- Traffic to `10.1.2.3` goes to Exit B (longer prefix)
- Traffic to `10.2.3.4` goes to Exit A

## DNS Configuration

### Public DNS

```yaml
exit:
  dns:
    servers:
      - "8.8.8.8:53"         # Google DNS
      - "1.1.1.1:53"         # Cloudflare DNS
    timeout: 5s
```

### Private DNS

For internal domains:

```yaml
exit:
  dns:
    servers:
      - "10.0.0.1:53"        # Internal DNS server
    timeout: 5s
```

### DNS-over-TLS (DoT)

Not currently supported. Use standard DNS.

### No DNS

If DNS is not configured, domain names will fail to resolve:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  # No dns section - only IP addresses work
```

## Access Control

Routes also serve as access control:

```yaml
exit:
  routes:
    - "10.0.0.0/8"          # Only allow internal network
    # No 0.0.0.0/0 = no internet access
```

Connections to non-matching destinations receive `STREAM_OPEN_ERR`.

## Examples

### Internet Gateway

Allow all traffic:

```yaml
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"           # All IPv4 traffic
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

### Private Network Only

Internal resources only:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
  dns:
    servers:
      - "10.0.0.1:53"       # Internal DNS
    timeout: 5s
```

### Specific Service Access

Only database and API servers:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.1.10/32"        # Database server
    - "10.0.1.20/32"        # API server
  dns:
    servers:
      - "10.0.0.1:53"
    timeout: 5s
```

### Split Horizon

Different exits for different networks:

**Exit A (internal network):**
```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  dns:
    servers:
      - "10.0.0.1:53"
```

**Exit B (internet):**
```yaml
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
```

Traffic is routed:
- `10.x.x.x` -> Exit A (longer prefix)
- Everything else -> Exit B (default route)

## Route Advertisement

Routes are advertised to the mesh:

- **Automatically**: Every `routing.advertise_interval` (default 2m)
- **Manually**: Via HTTP API

### Trigger Immediate Advertisement

```bash
curl -X POST http://localhost:8080/routes/advertise
```

Use after:
- Configuration changes
- Network changes
- Agent restart

### Routing Configuration

```yaml
routing:
  advertise_interval: 2m    # How often to re-advertise
  route_ttl: 5m             # Route expiration time
  max_hops: 16              # Maximum path length
```

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_exit_connections_active` | Gauge | Active connections |
| `muti_metroo_exit_connections_total` | Counter | Total connections |
| `muti_metroo_exit_dns_queries_total` | Counter | DNS queries |
| `muti_metroo_exit_dns_latency_seconds` | Histogram | DNS latency |
| `muti_metroo_exit_errors_total` | Counter | Errors by type |

## Troubleshooting

### No Route Found

```
Error: no route to 1.2.3.4
```

- Check exit is enabled
- Verify routes include destination
- Check exit agent is connected to mesh

### DNS Resolution Failed

```
Error: DNS lookup failed for example.com
```

- Verify DNS servers are reachable
- Check DNS timeout
- Test DNS directly: `dig @8.8.8.8 example.com`

### Connection Refused

```
Error: connection refused to 10.0.0.5:22
```

- Verify destination is reachable from exit agent
- Check firewall rules on exit host
- Test directly: `nc -zv 10.0.0.5 22`

### Access Denied

```
Error: destination not in allowed routes
```

- Add appropriate route to `exit.routes`
- Use more permissive CIDR (e.g., `/8` instead of `/24`)

## Security Considerations

1. **Principle of least privilege**: Only advertise necessary routes
2. **Avoid `0.0.0.0/0`** unless you need full internet access
3. **Use internal DNS** for private networks
4. **Monitor exit traffic** via metrics
5. **Consider network segmentation**: Different exits for different trust levels

## Related

- [Features: Exit Routing](../features/exit-routing) - Detailed usage
- [Concepts: Routing](../concepts/routing) - How routing works
- [Security: Access Control](../security/access-control) - Security best practices
