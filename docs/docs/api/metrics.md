---
title: Metrics Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole exposing metrics" style={{maxWidth: '180px'}} />
</div>

# Metrics Endpoints

Prometheus metrics exposure.

## GET /metrics

Local agent Prometheus metrics.

**Response:** Prometheus text format

```
# HELP muti_metroo_peers_connected Currently connected peers
# TYPE muti_metroo_peers_connected gauge
muti_metroo_peers_connected 3

# HELP muti_metroo_streams_active Currently active streams
# TYPE muti_metroo_streams_active gauge
muti_metroo_streams_active 42
...
```

## GET /metrics/\{agent-id\}

Fetch Prometheus metrics from remote agent.

**Parameters:**
- `agent-id`: Target agent ID

**Response:** Prometheus text format from remote agent

**Example:**
```bash
curl http://localhost:8080/metrics/abc123def456
```

This fetches metrics from agent `abc123def456` via the control channel.

## Available Metrics

See [Metrics and Monitoring](../features/metrics-monitoring) for complete list.

### Key Metrics

- `muti_metroo_peers_connected`: Connected peers
- `muti_metroo_streams_active`: Active streams
- `muti_metroo_routes_total`: Routes in table
- `muti_metroo_bytes_sent_total`: Bytes sent
- `muti_metroo_bytes_received_total`: Bytes received
- `muti_metroo_socks5_connections_active`: Active SOCKS5 connections
- `muti_metroo_exit_connections_active`: Active exit connections

## Scraping Configuration

Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: 'muti-metroo'
    static_configs:
      - targets:
        - 'agent1:8080'
        - 'agent2:8080'
        - 'agent3:8080'
```
