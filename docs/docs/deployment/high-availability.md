---
title: High Availability
sidebar_position: 5
---

# High Availability

Design patterns for fault-tolerant Muti Metroo deployments.

## Overview

High availability in Muti Metroo is achieved through:

1. **Redundant paths**: Multiple routes to destinations
2. **Automatic failover**: Route selection based on availability
3. **Reconnection**: Automatic peer reconnection with backoff
4. **Load distribution**: Multiple exit points

## Pattern 1: Redundant Transit

Multiple transit paths between ingress and exit.

### Architecture

```
                    +-------------+
                    |   Transit   |
          +-------> |   Primary   | --------+
          |         +-------------+         |
          |                                 v
+-------------+                        +-------------+
|   Ingress   |                        |    Exit     |
|    Agent    |                        |    Agent    |
+-------------+                        +-------------+
          |                                 ^
          |         +-------------+         |
          +-------> |   Transit   | --------+
                    |   Backup    |
                    +-------------+
```

### Configuration

**Ingress Agent:**

```yaml
agent:
  display_name: "Ingress"

peers:
  # Primary transit
  - id: "${PRIMARY_TRANSIT_ID}"
    transport: quic
    address: "transit-primary.example.com:4433"
    tls:
      ca: "./certs/ca.crt"

  # Backup transit
  - id: "${BACKUP_TRANSIT_ID}"
    transport: quic
    address: "transit-backup.example.com:4433"
    tls:
      ca: "./certs/ca.crt"

socks5:
  enabled: true
  address: "127.0.0.1:1080"
```

Both transits connect to the exit, which advertises routes. The ingress learns routes via both paths and prefers the one with lower hop count.

### Failover Behavior

1. Primary transit fails
2. Routes via primary expire (TTL, default 5m)
3. Only backup route remains
4. Traffic flows through backup
5. Primary recovers - routes re-advertise
6. Traffic returns to primary (lower metric)

### Faster Failover

Reduce route TTL for faster detection:

```yaml
routing:
  route_ttl: 1m              # 1 minute TTL
  advertise_interval: 30s    # Advertise every 30s
```

## Pattern 2: Multiple Exits

Multiple exit points for the same routes.

### Architecture

```
+-------------+         +-------------+
|   Ingress   |         |   Exit A    |  10.0.0.0/8
|    Agent    | ------> |   (DC East) |
+-------------+         +-------------+
        |
        |               +-------------+
        +-------------> |   Exit B    |  10.0.0.0/8
                        |   (DC West) |
                        +-------------+
```

### Configuration

**Exit A:**

```yaml
agent:
  display_name: "Exit A (DC East)"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  dns:
    servers:
      - "10.0.0.1:53"
```

**Exit B:**

```yaml
agent:
  display_name: "Exit B (DC West)"

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  dns:
    servers:
      - "10.0.0.1:53"
```

The ingress learns the same route from both exits and uses the one with lower metric.

## Pattern 3: Geographic Redundancy

Agents in multiple regions for disaster recovery.

### Architecture

```
              Region A                           Region B
         +--------------+                   +--------------+
         |   Agent A1   |                   |   Agent B1   |
         |   (Primary)  | <---------------> |   (Backup)   |
         +--------------+                   +--------------+
              |    |                             |    |
         +----+    +----+                   +----+    +----+
         |              |                   |              |
    +--------+    +--------+           +--------+    +--------+
    | Exit A |    | Exit A'|           | Exit B |    | Exit B'|
    +--------+    +--------+           +--------+    +--------+
```

### Cross-Region Peering

```yaml
# Agent A1 (Region A)
agent:
  display_name: "Region A Primary"

peers:
  # Local exit
  - id: "${EXIT_A_ID}"
    transport: quic
    address: "exit-a.region-a.internal:4433"

  # Cross-region peer
  - id: "${AGENT_B1_ID}"
    transport: quic
    address: "agent-b1.region-b.example.com:4433"
```

## Pattern 4: Active-Active Ingress

Multiple ingress points behind a load balancer.

### Architecture

```
                    +---------------+
                    | Load Balancer |
                    | (DNS/L4)      |
                    +-------+-------+
                            |
            +---------------+---------------+
            |                               |
      +-----+-----+                   +-----+-----+
      |  Ingress  |                   |  Ingress  |
      |  Agent 1  |                   |  Agent 2  |
      +-----+-----+                   +-----+-----+
            |                               |
            +---------------+---------------+
                            |
                      +-----+-----+
                      |   Exit    |
                      |   Agent   |
                      +-----------+
```

### DNS Round-Robin

```
proxy.example.com.  300  IN  A  192.168.1.10  # Ingress 1
proxy.example.com.  300  IN  A  192.168.1.11  # Ingress 2
```

### L4 Load Balancer

HAProxy example:

```
frontend socks5
    bind *:1080
    mode tcp
    default_backend socks5_backends

backend socks5_backends
    mode tcp
    balance roundrobin
    server ingress1 192.168.1.10:1080 check
    server ingress2 192.168.1.11:1080 check
```

## Monitoring for HA

### Health Checks

```bash
# Check each agent
curl http://ingress1:8080/health
curl http://ingress2:8080/health
curl http://exit:8080/health

# Check peer connectivity
curl http://ingress1:8080/healthz | jq '.peers'
```

### Key Metrics

Monitor these for HA:

| Metric | Alert Condition |
|--------|-----------------|
| `peers_connected` | < expected count |
| `routes_total` | < expected count |
| `peer_disconnects_total` | Spike in rate |
| `stream_errors_total` | Spike in rate |

### Prometheus Alert Rules

```yaml
groups:
  - name: muti-metroo
    rules:
      - alert: PeerDisconnected
        expr: muti_metroo_peers_connected < 2
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Muti Metroo has fewer than 2 peers connected"

      - alert: NoRoutes
        expr: muti_metroo_routes_total == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Muti Metroo has no routes in routing table"
```

## Reconnection Tuning

Configure aggressive reconnection for faster recovery:

```yaml
connections:
  reconnect:
    initial_delay: 500ms      # Start fast
    max_delay: 30s            # Cap at 30s
    multiplier: 1.5           # Slower backoff
    jitter: 0.3               # 30% jitter
    max_retries: 0            # Never give up
```

## Best Practices

1. **Minimum two paths**: Always have redundant routes
2. **Geographic diversity**: Spread agents across regions
3. **Independent failure domains**: Different networks, power, etc.
4. **Monitor everything**: Alerts before users notice
5. **Test failover**: Regularly test by killing components
6. **Document topology**: Know what depends on what

## Testing Failover

### Manual Testing

```bash
# Kill a transit agent
docker stop transit-primary

# Verify traffic still flows
curl -x socks5://localhost:1080 https://example.com

# Check routes updated
curl http://localhost:8080/healthz | jq '.routes'

# Bring transit back
docker start transit-primary

# Verify primary route restored
sleep 120  # Wait for advertisement
curl http://localhost:8080/healthz | jq '.routes'
```

### Chaos Testing

Use the built-in chaos package for automated testing:

```go
// internal/chaos provides fault injection
chaosMonkey := chaos.NewChaosMonkey(agent)
chaosMonkey.InjectNetworkDelay(100 * time.Millisecond)
chaosMonkey.DisconnectRandomPeer()
```

## Next Steps

- [Monitoring](../features/metrics-monitoring) - Set up monitoring
- [Troubleshooting](../troubleshooting/connectivity) - Debug connectivity
- [Deployment Scenarios](scenarios) - More deployment patterns
