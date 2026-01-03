---
title: Routing
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-plumbing.png" alt="Mole routing pipes" style={{maxWidth: '180px'}} />
</div>

# Routing in Muti Metroo

Muti Metroo automatically discovers and selects the best routes through the mesh network.

## Overview

Routing works as follows:

1. **Exit agents** advertise which networks they can reach (CIDR routes)
2. **Routes propagate** automatically through the mesh
3. **Each agent** maintains a routing table
4. **Traffic is routed** to the exit agent that can reach the destination

## Configuring Exit Routes

Exit agents advertise their available routes:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"        # Internal network
    - "192.168.0.0/16"    # Private network
    - "0.0.0.0/0"         # Default route (all destinations)
```

Each route includes:
- **CIDR**: Network prefix (e.g., `10.0.0.0/8`)
- **Metric**: Hop count (automatically calculated)

## Route Selection

When multiple routes match a destination, the **most specific route wins** (longest prefix match):

| CIDR | Next Hop | Metric |
|------|----------|--------|
| 1.2.3.4/32 | Agent A | 3 |
| 1.2.3.0/24 | Agent B | 2 |
| 0.0.0.0/0 | Agent C | 1 |

Lookups:

| Destination | Winner | Reason |
|-------------|--------|--------|
| 1.2.3.4 | Agent A | /32 is most specific |
| 1.2.3.100 | Agent B | /24 is more specific than /0 |
| 8.8.8.8 | Agent C | Only /0 matches |

If multiple routes have the same prefix length, the one with the **lowest metric** (fewest hops) wins.

## Route Expiration

Routes expire if not refreshed:

```yaml
routing:
  route_ttl: 5m              # Routes expire after 5 minutes
  advertise_interval: 2m     # Re-advertise every 2 minutes
```

If an agent goes offline, its routes will expire after `route_ttl`.

## Manual Route Trigger

Force immediate route advertisement after configuration changes:

```bash
curl -X POST http://localhost:8080/routes/advertise
```

## Redundant Paths

Multiple agents can advertise the same route for redundancy:

- Both routes are stored
- Traffic uses the route with the lowest metric
- If one agent disconnects, traffic automatically switches to the other

## Viewing Routes

### CLI

```bash
# View local routes via HTTP API
muti-metroo routes
```

### HTTP API

```bash
# View local agent stats (peer count, route count)
curl http://localhost:8080/healthz

# View routes from specific agent
curl http://localhost:8080/agents/{agent-id}/routes
```

### Dashboard

Access the web dashboard at `http://localhost:8080/ui/` to see all routes.

## Configuration Reference

```yaml
routing:
  # How often to re-advertise routes
  advertise_interval: 2m

  # How often to advertise node info
  node_info_interval: 2m

  # Time until routes expire without refresh
  route_ttl: 5m

  # Maximum path length (hops)
  max_hops: 16
```

## Troubleshooting

### No Route to Host

```bash
# Check if route exists
curl http://localhost:8080/healthz | jq '.routes'

# Verify peer is connected
curl http://localhost:8080/healthz | jq '.peers'

# Check exit agent is advertising
curl http://exit-agent:8080/healthz
```

### Route Not Appearing

Common causes:
- Agent not connected as a peer
- Route TTL expired
- Max hops exceeded

Debug:
```bash
# Enable debug logging
muti-metroo run -c config.yaml --log-level debug
```

## Best Practices

1. **Use specific routes**: Prefer `/24` over `/0` when possible
2. **Monitor route count**: High route counts increase memory usage
3. **Set appropriate TTL**: Balance responsiveness vs. stability
4. **Limit max hops**: Match your actual topology depth

## Next Steps

- [Streams](streams) - How data flows through routes
- [Exit Routing](../features/exit-routing) - Configure exit routes
- [Troubleshooting Connectivity](../troubleshooting/connectivity) - Debug routing issues
