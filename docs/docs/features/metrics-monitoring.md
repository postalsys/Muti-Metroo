---
title: Metrics and Monitoring
sidebar_position: 6
---

# Metrics and Monitoring

Muti Metroo exposes Prometheus metrics and provides comprehensive monitoring capabilities for production deployments.

## Prometheus Metrics Endpoint

Metrics are exposed at the `/metrics` endpoint:

```bash
curl http://localhost:8080/metrics
```

All metrics are prefixed with `muti_metroo_`.

## Connection Metrics

Monitor peer connectivity and connection health:

| Metric | Type | Description |
|--------|------|-------------|
| `peers_connected` | Gauge | Currently connected peers |
| `peers_total` | Counter | Total peer connections established |
| `peer_connections_total` | Counter | Connections by transport and direction |
| `peer_disconnects_total` | Counter | Disconnections by reason |

**Labels:**
- `peer_connections_total`: `transport` (quic, h2, ws), `direction` (inbound, outbound)
- `peer_disconnects_total`: `reason` (timeout, error, shutdown)

### Example Queries

```promql
# Current peer count
muti_metroo_peers_connected

# Connection rate by transport
rate(muti_metroo_peer_connections_total[5m])

# Disconnect rate
rate(muti_metroo_peer_disconnects_total[5m])
```

## Stream Metrics

Monitor stream lifecycle and performance:

| Metric | Type | Description |
|--------|------|-------------|
| `streams_active` | Gauge | Currently active streams |
| `streams_opened_total` | Counter | Total streams opened |
| `streams_closed_total` | Counter | Total streams closed |
| `stream_open_latency_seconds` | Histogram | Stream open latency |
| `stream_errors_total` | Counter | Stream errors by type |

**Labels:**
- `stream_errors_total`: `error_type` (timeout, rejected, reset)

### Example Queries

```promql
# Active stream count
muti_metroo_streams_active

# Stream open rate
rate(muti_metroo_streams_opened_total[5m])

# P99 stream open latency
histogram_quantile(0.99, rate(muti_metroo_stream_open_latency_seconds_bucket[5m]))

# Error rate by type
rate(muti_metroo_stream_errors_total[5m])
```

## Data Transfer Metrics

Monitor throughput and data volume:

| Metric | Type | Description |
|--------|------|-------------|
| `bytes_sent_total` | Counter | Bytes sent by type |
| `bytes_received_total` | Counter | Bytes received by type |
| `frames_sent_total` | Counter | Frames sent by type |
| `frames_received_total` | Counter | Frames received by type |

**Labels:**
- `bytes_*`: `type` (data, control)
- `frames_*`: `frame_type` (stream_data, stream_open, keepalive, etc.)

### Example Queries

```promql
# Data throughput (bytes/sec)
rate(muti_metroo_bytes_sent_total{type="data"}[5m])

# Frame rate by type
rate(muti_metroo_frames_sent_total[5m])
```

## Routing Metrics

Monitor route table and propagation:

| Metric | Type | Description |
|--------|------|-------------|
| `routes_total` | Gauge | Routes in routing table |
| `route_advertises_total` | Counter | Route advertisements processed |
| `route_withdrawals_total` | Counter | Route withdrawals processed |
| `route_flood_latency_seconds` | Histogram | Route flood propagation latency |

### Example Queries

```promql
# Route table size
muti_metroo_routes_total

# Route advertisement rate
rate(muti_metroo_route_advertises_total[5m])

# Route propagation latency
histogram_quantile(0.95, rate(muti_metroo_route_flood_latency_seconds_bucket[5m]))
```

## SOCKS5 Metrics

Monitor proxy server performance:

| Metric | Type | Description |
|--------|------|-------------|
| `socks5_connections_active` | Gauge | Active SOCKS5 connections |
| `socks5_connections_total` | Counter | Total SOCKS5 connections |
| `socks5_auth_failures_total` | Counter | Authentication failures |
| `socks5_connect_latency_seconds` | Histogram | Connect request latency |

### Example Queries

```promql
# Active SOCKS5 connections
muti_metroo_socks5_connections_active

# Auth failure rate
rate(muti_metroo_socks5_auth_failures_total[5m])

# P95 connect latency
histogram_quantile(0.95, rate(muti_metroo_socks5_connect_latency_seconds_bucket[5m]))
```

## Exit Handler Metrics

Monitor exit node operations:

| Metric | Type | Description |
|--------|------|-------------|
| `exit_connections_active` | Gauge | Active exit connections |
| `exit_connections_total` | Counter | Total exit connections |
| `exit_dns_queries_total` | Counter | DNS queries performed |
| `exit_dns_latency_seconds` | Histogram | DNS query latency |
| `exit_errors_total` | Counter | Exit errors by type |

**Labels:**
- `exit_errors_total`: `error_type` (dial_failed, dns_failed, timeout)

### Example Queries

```promql
# Exit connection count
muti_metroo_exit_connections_active

# DNS query rate
rate(muti_metroo_exit_dns_queries_total[5m])

# DNS latency
histogram_quantile(0.99, rate(muti_metroo_exit_dns_latency_seconds_bucket[5m]))
```

## Protocol Metrics

Monitor protocol-level operations:

| Metric | Type | Description |
|--------|------|-------------|
| `handshake_latency_seconds` | Histogram | Peer handshake latency |
| `handshake_errors_total` | Counter | Handshake errors by type |
| `keepalives_sent_total` | Counter | Keepalives sent |
| `keepalives_received_total` | Counter | Keepalives received |
| `keepalive_rtt_seconds` | Histogram | Keepalive round-trip time |

### Example Queries

```promql
# Handshake latency
histogram_quantile(0.99, rate(muti_metroo_handshake_latency_seconds_bucket[5m]))

# Keepalive RTT
histogram_quantile(0.95, rate(muti_metroo_keepalive_rtt_seconds_bucket[5m]))
```

## RPC Metrics

Monitor remote command execution:

| Metric | Type | Description |
|--------|------|-------------|
| `rpc_calls_total` | Counter | RPC calls by result and command |
| `rpc_call_duration_seconds` | Histogram | RPC call duration |
| `rpc_bytes_received_total` | Counter | Bytes received in requests |
| `rpc_bytes_sent_total` | Counter | Bytes sent in responses |

**Labels:**
- `rpc_calls_total`: `result` (success, failed, rejected, auth_failed), `command`

## Alerting Examples

Example Prometheus alert rules:

```yaml
groups:
  - name: muti-metroo
    rules:
      # No connected peers
      - alert: NoPeersConnected
        expr: muti_metroo_peers_connected == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "No peers connected on {{ $labels.instance }}"

      # High stream error rate
      - alert: HighStreamErrors
        expr: rate(muti_metroo_stream_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High stream error rate on {{ $labels.instance }}"

      # High SOCKS5 auth failures
      - alert: HighAuthFailures
        expr: rate(muti_metroo_socks5_auth_failures_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High auth failure rate - possible brute force"

      # High latency
      - alert: HighStreamLatency
        expr: histogram_quantile(0.99, rate(muti_metroo_stream_open_latency_seconds_bucket[5m])) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High stream open latency on {{ $labels.instance }}"

      # Route table empty
      - alert: NoRoutes
        expr: muti_metroo_routes_total == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "No routes in routing table on {{ $labels.instance }}"
```

## Grafana Dashboard

Example Grafana dashboard JSON available in the repository.

### Key Panels

1. **Overview**: Peer count, stream count, route count
2. **Throughput**: Bytes/sec sent and received
3. **Latency**: Stream open, keepalive RTT, DNS
4. **Errors**: Stream errors, auth failures, disconnects
5. **Resources**: Memory usage, goroutine count

### Dashboard Variables

- `$instance`: Filter by Prometheus instance
- `$job`: Filter by job name

## Health Endpoints

In addition to Prometheus metrics:

```bash
# Basic health check
curl http://localhost:8080/health
# Returns: OK

# Detailed health with stats
curl http://localhost:8080/healthz | jq
# Returns JSON with peer count, stream count, etc.

# Kubernetes readiness
curl http://localhost:8080/ready
# Returns: OK when ready to serve traffic
```

## Related

- [HTTP API - Metrics](../api/metrics) - API reference
- [Deployment - Docker](../deployment/docker) - Docker compose with Prometheus
- [Troubleshooting - Performance](../troubleshooting/performance) - Performance tuning
