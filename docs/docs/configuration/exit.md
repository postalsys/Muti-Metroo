---
title: Exit
sidebar_position: 6
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring exit" style={{maxWidth: '180px'}} />
</div>

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
  domain_routes:
    - "api.internal.corp"
    - "*.example.com"
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
| `domain_routes` | array | [] | Domain patterns to advertise |
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
    - "0.0.0.0/0"          # Default route (all IPv4 traffic)
```

### Route Types

| Route | Description |
|-------|-------------|
| `10.0.0.0/8` | Internal network |
| `192.168.1.0/24` | Specific subnet |
| `1.2.3.4/32` | Single host |
| `0.0.0.0/0` | Default route (all IPv4) |
| `2001:db8::/32` | IPv6 documentation prefix |
| `fd00::/8` | IPv6 unique local addresses |
| `::1/128` | IPv6 single host (localhost) |
| `::/0` | Default route (all IPv6) |

### IPv6 Routes

Muti Metroo fully supports IPv6 routes. Use standard CIDR notation:

```yaml
exit:
  routes:
    # IPv4 routes
    - "10.0.0.0/8"
    - "0.0.0.0/0"
    # IPv6 routes
    - "2001:db8::/32"      # Specific IPv6 prefix
    - "fd00::/8"           # Unique local addresses
    - "::/0"               # Default route (all IPv6)
```

For dual-stack environments, include both IPv4 and IPv6 default routes:

```yaml
exit:
  routes:
    - "0.0.0.0/0"          # All IPv4 traffic
    - "::/0"               # All IPv6 traffic
```

### Route Selection

When multiple exit nodes advertise overlapping routes:

1. **Longest prefix wins**: `/32` beats `/24` beats `/0`
2. **Lower metric wins**: Closer exit preferred

Example:
- Exit A advertises `10.0.0.0/8` (metric 1)
- Exit B advertises `10.1.0.0/16` (metric 2)
- Traffic to `10.1.2.3` goes to Exit B (longer prefix)
- Traffic to `10.2.3.4` goes to Exit A

## Domain Routes

Domain routes allow routing based on domain names instead of IP addresses. When a SOCKS5 client requests a connection to a domain matching a domain route, the domain is passed to the exit node for DNS resolution instead of being resolved at the ingress.

```yaml
exit:
  domain_routes:
    - "api.internal.corp"      # Exact match
    - "*.example.com"          # Wildcard match
    - "*.prod.service.local"   # Wildcard for specific subdomain
```

### Domain Pattern Types

| Pattern | Matches | Does NOT Match |
|---------|---------|----------------|
| `api.example.com` | `api.example.com` | `foo.api.example.com` |
| `*.example.com` | `foo.example.com`, `bar.example.com` | `example.com`, `a.b.example.com` |
| `*.api.example.com` | `foo.api.example.com` | `api.example.com`, `a.b.api.example.com` |

### Wildcard Matching

Wildcards use **single-level matching** only:

- `*.example.com` matches `foo.example.com` and `bar.example.com`
- `*.example.com` does **NOT** match `a.b.example.com` (multi-level subdomain)
- `*.example.com` does **NOT** match `example.com` (base domain)

### Domain vs CIDR Priority

When both domain and CIDR routes exist:

1. **Domain routes are checked first** for domain-based requests
2. If no domain route matches, DNS resolution happens at the ingress
3. Then **CIDR routes** are used based on the resolved IP

### Multiple Origins

Multiple exit agents can advertise the same domain pattern. The route with the **lowest metric** (fewest hops) is selected.

### Use Cases

Domain routes are ideal for:

- **Split-horizon DNS**: Internal domains resolved by internal DNS servers
- **Private services**: Route `*.internal.corp` to an internal exit
- **Geo-specific resolution**: Different DNS results based on exit location

## DNS Configuration

:::info DNS Resolution Behavior
DNS resolution location depends on the route type:

- **CIDR routes**: Domain names are resolved at the **ingress agent** using the system's DNS resolver. The exit node receives IP addresses.
- **Domain routes**: Domain names are passed to the **exit node** for resolution using the configured DNS servers below.

If you use domain routes, configure the `dns` section to specify which DNS servers the exit node should use.
:::

### Public DNS

```yaml
exit:
  dns:
    servers:
      - "8.8.8.8:53"         # Google DNS (IPv4)
      - "1.1.1.1:53"         # Cloudflare DNS (IPv4)
    timeout: 5s
```

### IPv6 DNS Servers

IPv6 DNS servers are supported using bracket notation:

```yaml
exit:
  dns:
    servers:
      - "[2001:4860:4860::8888]:53"   # Google DNS (IPv6)
      - "[2606:4700:4700::1111]:53"   # Cloudflare DNS (IPv6)
    timeout: 5s
```

For dual-stack DNS resolution, include both IPv4 and IPv6 servers:

```yaml
exit:
  dns:
    servers:
      - "8.8.8.8:53"                   # Google DNS (IPv4)
      - "[2001:4860:4860::8888]:53"    # Google DNS (IPv6)
    timeout: 5s
```

:::note DNS Resolution Preference
When a domain resolves to both A (IPv4) and AAAA (IPv6) records, Muti Metroo prefers IPv4 addresses. If only AAAA records exist, IPv6 addresses are used.
:::

### Private DNS

For internal domains:

```yaml
exit:
  dns:
    servers:
      - "10.0.0.1:53"        # Internal DNS server (IPv4)
    timeout: 5s
```

Or with IPv6:

```yaml
exit:
  dns:
    servers:
      - "[fd00::1]:53"       # Internal DNS server (IPv6)
    timeout: 5s
```

### DNS-over-TLS (DoT)

Not currently supported. Use standard DNS.

### No DNS

If you only use CIDR routes (not domain routes), the `dns` section can be omitted since DNS resolution happens at the ingress agent:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  # dns section is optional when using only CIDR routes
```

If you use domain routes, you should configure DNS servers for the exit node to resolve domains.

## Access Control

Routes also serve as access control:

```yaml
exit:
  routes:
    - "10.0.0.0/8"          # Only allow internal network
    # No 0.0.0.0/0 = no internet access
```

Connections to non-matching destinations are rejected.

## Examples

### Internet Gateway (IPv4)

Allow all IPv4 traffic:

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

### Internet Gateway (Dual-Stack)

Allow both IPv4 and IPv6 traffic:

```yaml
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"           # All IPv4 traffic
    - "::/0"                # All IPv6 traffic
  dns:
    servers:
      - "8.8.8.8:53"                   # Google DNS (IPv4)
      - "[2001:4860:4860::8888]:53"    # Google DNS (IPv6)
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
- `10.x.x.x` → Exit A (longer prefix)
- Everything else → Exit B (default route)

### Domain-Based Routing

Route specific domains to an internal exit node:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  domain_routes:
    - "api.internal.corp"       # Exact match for API server
    - "*.internal.corp"         # All *.internal.corp subdomains
    - "*.prod.mycompany.local"  # Production services
  dns:
    servers:
      - "10.0.0.1:53"           # Internal DNS server
    timeout: 5s
```

Traffic is routed:
- `api.internal.corp` → This exit (exact domain match)
- `foo.internal.corp` → This exit (wildcard match)
- `bar.prod.mycompany.local` → This exit (wildcard match)
- Domain not matching any pattern → DNS resolved at ingress, then CIDR routing

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

DNS resolution location depends on route type:

**For CIDR routes** (domain resolved at ingress):
- Check DNS configuration on the ingress agent's host system
- Verify the ingress host can resolve domains: `dig example.com`
- Check `/etc/resolv.conf` on the ingress host

**For domain routes** (domain resolved at exit):
- Check `exit.dns.servers` configuration on the exit agent
- Verify the exit host can reach the DNS servers
- Test DNS resolution from the exit host: `dig @10.0.0.1 example.com`

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
4. **Consider network segmentation**: Different exits for different trust levels

## Related

- [Features: Exit Routing](../features/exit-routing) - Detailed usage
- [Concepts: Routing](../concepts/routing) - How routing works
- [Security: Access Control](../security/access-control) - Security best practices
