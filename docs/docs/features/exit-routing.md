---
title: Exit Routing
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting exit routing" style={{maxWidth: '180px'}} />
</div>

# Exit Routing

Control where your traffic goes. An exit node opens connections to destinations on your behalf - reach internal networks, specific IP ranges, or route by domain name.

**Common scenarios:**
- Route `10.0.0.0/8` through an exit inside a corporate network
- Route `*.internal.corp` to an agent with access to internal DNS
- Route `0.0.0.0/0` for a general-purpose exit to the internet

## Route Types

- **CIDR routes**: Match destinations by IP address (e.g., `10.0.0.0/8`)
- **Domain routes**: Match destinations by domain name (e.g., `*.example.com`)

:::note SOCKS5 vs TUN Interface
Domain routes only work with **SOCKS5 clients** that send the destination hostname. The [Mutiauk TUN interface](/mutiauk) operates at Layer 3 (IP) and only sees IP addresses after DNS resolution, so it can only use **CIDR routes**. Mutiauk's autoroutes feature fetches only CIDR routes from Muti Metroo.
:::

## Configuration

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "0.0.0.0/0"  # Default route
  domain_routes:
    - "api.internal.corp"    # Exact domain match
    - "*.example.com"        # Wildcard match
  dns:                       # Optional, defaults to Google (8.8.8.8) + Cloudflare (1.1.1.1)
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

## Route Advertisement

Routes are propagated through the mesh:

- **Periodic**: Every `routing.advertise_interval` (default 2m)
- **On-demand**: Via HTTP API `POST /routes/advertise`

### Trigger Immediate Advertisement

```bash
curl -X POST http://localhost:8080/routes/advertise
```

## DNS Resolution

DNS resolution location depends on the route type:

### CIDR Routes (DNS at Ingress)

For destinations matching CIDR routes:

1. Client connects via SOCKS5 with domain (e.g., example.com)
2. **Ingress agent** resolves domain using the system's DNS resolver
3. Ingress performs route lookup using the resolved IP address
4. Ingress opens a stream to the exit node with the **IP address**
5. Exit opens TCP connection to the destination IP

### Domain Routes (DNS at Exit)

For destinations matching domain routes:

1. Client connects via SOCKS5 with domain (e.g., api.internal.corp)
2. Ingress checks domain routes first
3. If a domain route matches, ingress opens a stream to the exit node with the **domain name**
4. **Exit agent** resolves domain using the configured DNS servers
5. Exit opens TCP connection to the resolved IP

:::tip When to Use Domain Routes
Domain routes are ideal for:
- **Split-horizon DNS**: Internal domains that resolve differently inside vs. outside the network
- **Private services**: Route `*.internal.corp` to an internal exit with access to internal DNS
- **Geo-specific resolution**: Different DNS results based on exit node location
:::

## Route Selection

### Domain Routes

Domain routes are checked **first** for domain-based requests:

1. **Exact match**: `api.example.com` matches only `api.example.com`
2. **Wildcard match**: `*.example.com` matches single-level subdomains like `foo.example.com`
3. If no domain route matches, fall back to CIDR routing

Wildcard matching is **single-level only**:
- `*.example.com` matches `foo.example.com` and `bar.example.com`
- `*.example.com` does NOT match `a.b.example.com` or `example.com`

### CIDR Routes

Uses longest-prefix match:

1. Filter routes where CIDR contains destination IP
2. Select route with longest prefix (most specific)
3. If tied, select lowest metric (hop count)

Example:
- `1.2.3.4/32` beats `1.2.3.0/24` for 1.2.3.4
- `1.2.3.0/24` beats `0.0.0.0/0` for 1.2.3.5

## Access Control

Only destinations matching advertised routes are allowed:

```yaml
exit:
  routes:
    - "10.0.0.0/8"  # Only allow 10.x.x.x
```

Connections to other IPs will be rejected.

## Related

- [Configuration - Exit](/configuration/exit) - Full configuration reference
- [Concepts - Agent Roles](/concepts/agent-roles) - Understanding exit role
- [Concepts - Routing](/concepts/routing) - How routes propagate
- [Security - Access Control](/security/access-control) - Route-based access control
