---
title: Routing Algorithm
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-thinking.png" alt="Mole planning routes" style={{maxWidth: '180px'}} />
</div>

# Routing Algorithm

How Muti Metroo discovers and selects routes through the mesh.

## Route Propagation (Flooding)

Routes are propagated through the mesh using a flood-based algorithm:

1. **Exit agent advertises route**
   - Sends ROUTE_ADVERTISE frame with CIDR
   - Includes metric (hop count) and TTL

2. **Transit agents forward advertisement**
   - Increment hop count (metric)
   - Add own AgentID to SeenBy list
   - Forward to all connected peers

3. **Loop prevention**
   - Don't forward to agents in SeenBy list
   - Discard if hop count > max_hops
   - Discard if TTL expired

4. **Route table update**
   - Add/update route in local table
   - Trigger route subscribers (stream manager)

## Route Selection (Longest-Prefix Match)

When opening a stream, routes are selected using longest-prefix match:

1. **Filter**: Find routes where CIDR contains destination IP
2. **Sort by prefix length**: Longest prefix (most specific) wins
3. **Tiebreaker**: If tied, select lowest metric (hop count)

### Example

Routes in table:
- `1.2.3.4/32` (metric 3, via agent-a)
- `1.2.3.0/24` (metric 2, via agent-b)
- `0.0.0.0/0` (metric 1, via agent-c)

For destination `1.2.3.4`:
- All three match
- `/32` wins (longest prefix)
- Route via agent-a

For destination `1.2.3.100`:
- `1.2.3.0/24` and `0.0.0.0/0` match
- `/24` wins (longer than `/0`)
- Route via agent-b

For destination `8.8.8.8`:
- Only `0.0.0.0/0` matches
- Route via agent-c (default route)

## Route Expiration

Routes have configurable TTL (default 5m):

- If no ROUTE_ADVERTISE received within TTL, route expires
- Exit agents re-advertise periodically (default 2m)
- Ensures stale routes are removed

## Triggering Advertisement

Immediate advertisement can be triggered:

```bash
curl -X POST http://localhost:8080/routes/advertise
```

Useful after:
- Configuration changes
- Adding new exit routes
- Network topology changes

## Redundant Paths

Multiple agents can advertise the same route:

- All routes are stored in table
- Lowest metric (hop count) is preferred
- Provides automatic failover if primary path fails

Example:
- Agent-A advertises `10.0.0.0/8` (metric 1)
- Agent-B advertises `10.0.0.0/8` (metric 2)
- Traffic goes via Agent-A (lower metric)
- If Agent-A fails, traffic switches to Agent-B

## Node Information

Agents also advertise node metadata via NODE_INFO_ADVERTISE:

- Agent ID and display name
- Hostname and IP address
- OS and architecture
- Uptime

This enables the web dashboard to show mesh topology.

## Related

- [Concepts - Routing](../concepts/routing) - High-level routing concepts
- [Features - Exit Routing](../features/exit-routing) - Exit node configuration
- [Configuration - Exit](../configuration/exit) - Exit configuration reference
- [API - Routes](../api/routes) - Route API endpoints
