---
title: Exit Routing
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting exit routing" style={{maxWidth: '180px'}} />
</div>

# Exit Routing

Exit nodes advertise CIDR routes and open TCP connections to external destinations.

## Configuration

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "0.0.0.0/0"  # Default route
  dns:
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

DNS resolution happens at the **ingress agent**, not at the exit node:

1. Client connects via SOCKS5 with domain (e.g., example.com)
2. **Ingress agent** resolves domain using the system's DNS resolver
3. Ingress performs route lookup using the resolved IP address
4. Ingress opens a stream to the exit node with the **IP address**
5. Exit opens TCP connection to the destination IP
6. Traffic flows bidirectionally through the mesh

:::note
The `exit.dns` configuration is reserved for future use but is not currently active for SOCKS5 traffic. Domain names are always resolved at the ingress agent using the host system's DNS configuration.
:::

## Route Selection

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

## Metrics

- `muti_metroo_exit_connections_active`: Active exit connections
- `muti_metroo_exit_connections_total`: Total exit connections
- `muti_metroo_exit_dns_queries_total`: DNS queries
- `muti_metroo_exit_dns_latency_seconds`: DNS latency
- `muti_metroo_exit_errors_total`: Exit errors

## Related

- [Configuration - Exit](../configuration/exit) - Full configuration reference
- [Concepts - Agent Roles](../concepts/agent-roles) - Understanding exit role
- [Concepts - Routing](../concepts/routing) - How routes propagate
- [Security - Access Control](../security/access-control) - Route-based access control
