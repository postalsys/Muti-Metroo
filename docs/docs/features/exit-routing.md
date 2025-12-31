---
title: Exit Routing
---

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

Routes are advertised via ROUTE_ADVERTISE frames:

- **Periodic**: Every `routing.advertise_interval` (default 2m)
- **On-demand**: Via HTTP API `POST /routes/advertise`

### Trigger Immediate Advertisement

```bash
curl -X POST http://localhost:8080/routes/advertise
```

## DNS Resolution

Exit nodes resolve domain names to IP addresses:

1. Client connects via SOCKS5 with domain (e.g., example.com)
2. Exit node receives STREAM_OPEN with domain
3. Exit performs DNS lookup using configured servers
4. Opens TCP connection to resolved IP
5. Returns STREAM_OPEN_ACK

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

Connections to other IPs will be rejected with STREAM_OPEN_ERR.

## Metrics

- `muti_metroo_exit_connections_active`: Active exit connections
- `muti_metroo_exit_connections_total`: Total exit connections
- `muti_metroo_exit_dns_queries_total`: DNS queries
- `muti_metroo_exit_dns_latency_seconds`: DNS latency
- `muti_metroo_exit_errors_total`: Exit errors
