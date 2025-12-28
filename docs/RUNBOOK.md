# Muti Metroo Operational Runbook

This document provides operational procedures for running, monitoring, and troubleshooting Muti Metroo mesh networking agents in production environments.

## Table of Contents

1. [Deployment](#deployment)
2. [Health Checks](#health-checks)
3. [Monitoring](#monitoring)
4. [Common Issues](#common-issues)
5. [Troubleshooting Procedures](#troubleshooting-procedures)
6. [Maintenance Operations](#maintenance-operations)
7. [Emergency Procedures](#emergency-procedures)

---

## Deployment

### Prerequisites

- Go 1.22+ (for building from source)
- TLS certificates (CA, server, and optionally client certificates)
- Network connectivity between peer agents
- Firewall rules allowing the configured transport ports

### Initial Setup

1. **Generate certificates** (for development/testing):
   ```bash
   ./muti-metroo cert ca -n "My CA" -o ./certs
   ./muti-metroo cert agent -n "agent-1" --ca ./certs/ca.crt --ca-key ./certs/ca.key -o ./certs
   ```

2. **Initialize agent identity**:
   ```bash
   ./muti-metroo init -d ./data
   ```

3. **Create configuration**:
   ```bash
   ./muti-metroo setup  # Interactive wizard
   # or manually create config.yaml
   ```

4. **Start the agent**:
   ```bash
   ./muti-metroo run -c ./config.yaml
   ```

### Service Installation

#### Linux (systemd)
```bash
sudo ./muti-metroo install -c /etc/muti-metroo/config.yaml
sudo systemctl status muti-metroo
sudo journalctl -u muti-metroo -f
```

#### Windows
```powershell
.\muti-metroo.exe install -c C:\muti-metroo\config.yaml
sc query muti-metroo
```

### Uninstallation

```bash
# Linux
sudo ./muti-metroo uninstall

# Windows
.\muti-metroo.exe uninstall
```

---

## Health Checks

### HTTP Endpoints

When `health.enabled: true` in configuration, the following endpoints are available:

| Endpoint | Description | Response |
|----------|-------------|----------|
| `/health` | Basic liveness probe | `200 OK` with `OK\n` |
| `/healthz` | Detailed health with stats | `200 OK` or `503 Service Unavailable` with JSON |
| `/ready` | Readiness probe | `200 OK` with `READY\n` or `503` with `NOT READY\n` |
| `/metrics` | Prometheus metrics | Prometheus exposition format |
| `/debug/pprof/` | Go profiling endpoints | pprof data |

### Health Check Examples

```bash
# Basic health check
curl http://localhost:8080/health

# Detailed health with stats
curl http://localhost:8080/healthz
# Response:
# {
#   "status": "healthy",
#   "running": true,
#   "peer_count": 3,
#   "stream_count": 12,
#   "route_count": 45,
#   "socks5_running": true,
#   "exit_handler_running": false
# }

# Kubernetes readiness probe
curl -f http://localhost:8080/ready
```

### Kubernetes Integration

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: muti-metroo
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 10
      periodSeconds: 30
    readinessProbe:
      httpGet:
        path: /ready
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
```

---

## Monitoring

### Prometheus Metrics

Key metrics to monitor:

#### Connection Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_peers_connected` | Gauge | Currently connected peers |
| `muti_metroo_peers_total` | Counter | Total peer connections established |
| `muti_metroo_peer_connections_total` | Counter | Connections by transport/direction |
| `muti_metroo_peer_disconnects_total` | Counter | Disconnections by reason |

#### Stream Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_streams_active` | Gauge | Currently active streams |
| `muti_metroo_streams_opened_total` | Counter | Total streams opened |
| `muti_metroo_streams_closed_total` | Counter | Total streams closed |
| `muti_metroo_stream_open_latency_seconds` | Histogram | Stream open latency |
| `muti_metroo_stream_errors_total` | Counter | Stream errors by type |

#### Data Transfer Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_bytes_sent_total` | Counter | Bytes sent by type |
| `muti_metroo_bytes_received_total` | Counter | Bytes received by type |
| `muti_metroo_frames_sent_total` | Counter | Frames sent by type |
| `muti_metroo_frames_received_total` | Counter | Frames received by type |

#### Protocol Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_handshake_latency_seconds` | Histogram | Handshake latency |
| `muti_metroo_handshake_errors_total` | Counter | Handshake errors by type |
| `muti_metroo_keepalives_sent_total` | Counter | Keepalive messages sent |
| `muti_metroo_keepalives_received_total` | Counter | Keepalive messages received |
| `muti_metroo_keepalive_rtt_seconds` | Histogram | Keepalive round-trip time |

#### Routing Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_routes_total` | Gauge | Routes in routing table |
| `muti_metroo_route_advertises_total` | Counter | Route advertisements processed |
| `muti_metroo_route_withdrawals_total` | Counter | Route withdrawals processed |

#### SOCKS5 Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_socks5_connections_active` | Gauge | Active SOCKS5 connections |
| `muti_metroo_socks5_connections_total` | Counter | Total SOCKS5 connections |
| `muti_metroo_socks5_auth_failures_total` | Counter | SOCKS5 auth failures |
| `muti_metroo_socks5_connect_latency_seconds` | Histogram | SOCKS5 connect latency |

#### Exit Handler Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `muti_metroo_exit_connections_active` | Gauge | Active exit connections |
| `muti_metroo_exit_connections_total` | Counter | Total exit connections |
| `muti_metroo_exit_dns_queries_total` | Counter | DNS queries performed |
| `muti_metroo_exit_dns_latency_seconds` | Histogram | DNS query latency |
| `muti_metroo_exit_errors_total` | Counter | Exit errors by type |

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'muti-metroo'
    static_configs:
      - targets:
        - 'agent1:8080'
        - 'agent2:8080'
        - 'agent3:8080'
    scrape_interval: 15s
```

### Alerting Rules (Prometheus)

```yaml
groups:
- name: muti-metroo
  rules:
  # No peers connected
  - alert: MutiMetrooNoPeers
    expr: muti_metroo_peers_connected == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Muti Metroo has no connected peers"
      description: "{{ $labels.instance }} has had no connected peers for 5 minutes"

  # High handshake error rate
  - alert: MutiMetrooHandshakeErrors
    expr: rate(muti_metroo_handshake_errors_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High handshake error rate"
      description: "{{ $labels.instance }} has handshake error rate > 0.1/s"

  # High stream error rate
  - alert: MutiMetrooStreamErrors
    expr: rate(muti_metroo_stream_errors_total[5m]) > 1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High stream error rate"
      description: "{{ $labels.instance }} has stream error rate > 1/s"

  # SOCKS5 auth failures
  - alert: MutiMetrooSOCKS5AuthFailures
    expr: rate(muti_metroo_socks5_auth_failures_total[5m]) > 0.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High SOCKS5 auth failure rate"
      description: "{{ $labels.instance }} has SOCKS5 auth failure rate > 0.5/s"

  # Service down
  - alert: MutiMetrooDown
    expr: up{job="muti-metroo"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Muti Metroo instance down"
      description: "{{ $labels.instance }} is not responding"
```

### Grafana Dashboard Queries

**Connected Peers Over Time:**
```promql
muti_metroo_peers_connected
```

**Connection Rate:**
```promql
rate(muti_metroo_peer_connections_total[5m])
```

**Active Streams:**
```promql
muti_metroo_streams_active
```

**Data Transfer Rate:**
```promql
rate(muti_metroo_bytes_sent_total[5m]) + rate(muti_metroo_bytes_received_total[5m])
```

**Handshake Latency (p99):**
```promql
histogram_quantile(0.99, rate(muti_metroo_handshake_latency_seconds_bucket[5m]))
```

---

## Common Issues

### Issue: Agent Not Starting

**Symptoms:**
- Agent exits immediately after start
- Error messages in logs

**Common Causes and Solutions:**

1. **Config file not found:**
   ```bash
   ./muti-metroo run -c ./config.yaml
   # Ensure file exists and is readable
   ```

2. **Invalid config syntax:**
   ```bash
   # Validate YAML syntax
   cat config.yaml | python3 -c "import yaml,sys; yaml.safe_load(sys.stdin)"
   ```

3. **TLS certificate issues:**
   ```bash
   # Verify certificates
   openssl x509 -in ./certs/server.crt -text -noout
   openssl verify -CAfile ./certs/ca.crt ./certs/server.crt
   ```

4. **Port already in use:**
   ```bash
   # Check port usage
   netstat -tlnp | grep 4433
   lsof -i :4433
   ```

5. **Data directory permissions:**
   ```bash
   ls -la ./data
   # Ensure directory is writable
   ```

### Issue: Peers Not Connecting

**Symptoms:**
- `peers_connected` metric is 0
- Connection timeout errors in logs

**Troubleshooting Steps:**

1. **Verify network connectivity:**
   ```bash
   # Test basic connectivity
   nc -vz peer.example.com 4433

   # For QUIC (UDP)
   nc -vzu peer.example.com 4433
   ```

2. **Check firewall rules:**
   ```bash
   # Linux
   iptables -L -n
   ufw status

   # Check if UDP is allowed for QUIC
   ```

3. **Verify TLS configuration:**
   ```bash
   # Test TLS connection (TCP transports)
   openssl s_client -connect peer.example.com:4433
   ```

4. **Check peer ID configuration:**
   - Ensure `peers[].id` matches the remote agent's actual ID
   - Use `id: "auto"` to accept any peer ID

5. **Review logs for handshake errors:**
   ```bash
   journalctl -u muti-metroo | grep -i handshake
   ```

### Issue: SOCKS5 Connections Failing

**Symptoms:**
- Clients can't connect to SOCKS5 proxy
- Connection refused or timeout

**Troubleshooting Steps:**

1. **Verify SOCKS5 is enabled and running:**
   ```bash
   curl http://localhost:8080/healthz | jq .socks5_running
   ```

2. **Check SOCKS5 binding address:**
   ```yaml
   socks5:
     enabled: true
     address: "127.0.0.1:1080"  # Only localhost
     # or
     address: "0.0.0.0:1080"    # All interfaces
   ```

3. **Test SOCKS5 connectivity:**
   ```bash
   curl -x socks5://localhost:1080 http://example.com
   ```

4. **Check authentication:**
   ```bash
   # If auth enabled
   curl -x socks5://user:pass@localhost:1080 http://example.com
   ```

5. **Verify routes exist:**
   - Ensure exit node is connected and advertising routes
   - Check routing table via metrics

### Issue: High Latency

**Symptoms:**
- Slow data transfer
- High RTT in keepalive metrics

**Troubleshooting Steps:**

1. **Check network path:**
   ```bash
   traceroute peer.example.com
   mtr peer.example.com
   ```

2. **Review hop count:**
   - Check if traffic is taking suboptimal multi-hop routes
   - Review `routing.max_hops` configuration

3. **Monitor buffer sizes:**
   ```yaml
   limits:
     buffer_size: 262144  # Adjust if needed
   ```

4. **Check system resources:**
   ```bash
   top
   iostat
   vmstat
   ```

---

## Troubleshooting Procedures

### Collecting Debug Information

```bash
# Get current status
curl http://localhost:8080/healthz

# Get all metrics
curl http://localhost:8080/metrics

# Get CPU profile (30 seconds)
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Get heap profile
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Get goroutine stack traces
curl http://localhost:8080/debug/pprof/goroutine?debug=2

# View service logs
journalctl -u muti-metroo -n 1000 --no-pager
```

### Enabling Debug Logging

```yaml
agent:
  log_level: debug  # Change from info to debug
  log_format: json  # Easier to parse
```

Then restart:
```bash
systemctl restart muti-metroo
```

### Testing Connectivity Between Agents

From Agent A to Agent B:
```bash
# Check if Agent B is reachable
nc -vz agent-b.example.com 4433

# Verify TLS handshake works
openssl s_client -connect agent-b.example.com:4433 \
  -CAfile /path/to/ca.crt \
  -cert /path/to/client.crt \
  -key /path/to/client.key
```

### Verifying Route Propagation

1. Check routes on ingress agent
2. Verify exit agent is connected
3. Check route count in metrics
4. Review flood propagation in debug logs

---

## Maintenance Operations

### Rolling Restart

For zero-downtime restarts across a mesh:

1. Restart agents one at a time
2. Wait for peer reconnection between restarts
3. Verify health before proceeding to next agent

```bash
# On each agent
systemctl restart muti-metroo
sleep 30
curl -f http://localhost:8080/ready
```

### Certificate Rotation

1. Generate new certificates:
   ```bash
   ./muti-metroo cert agent -n "agent-1" \
     --ca ./certs/ca.crt \
     --ca-key ./certs/ca.key \
     -o ./certs/new
   ```

2. Update configuration to point to new certificates

3. Restart agent:
   ```bash
   systemctl restart muti-metroo
   ```

4. Verify connections:
   ```bash
   curl http://localhost:8080/healthz | jq .peer_count
   ```

### Backup and Restore

**Backup:**
```bash
# Backup identity and configuration
tar -czvf muti-metroo-backup.tar.gz \
  ./data/identity.json \
  ./config.yaml \
  ./certs/
```

**Restore:**
```bash
tar -xzvf muti-metroo-backup.tar.gz
./muti-metroo run -c ./config.yaml
```

### Upgrading

1. Stop the service:
   ```bash
   systemctl stop muti-metroo
   ```

2. Backup current binary:
   ```bash
   cp /usr/local/bin/muti-metroo /usr/local/bin/muti-metroo.bak
   ```

3. Install new binary:
   ```bash
   cp ./muti-metroo-new /usr/local/bin/muti-metroo
   chmod +x /usr/local/bin/muti-metroo
   ```

4. Start and verify:
   ```bash
   systemctl start muti-metroo
   curl -f http://localhost:8080/ready
   ```

---

## Emergency Procedures

### Complete Service Failure

1. **Check if process is running:**
   ```bash
   systemctl status muti-metroo
   ps aux | grep muti-metroo
   ```

2. **Check system resources:**
   ```bash
   df -h
   free -m
   dmesg | tail -50
   ```

3. **Review recent logs:**
   ```bash
   journalctl -u muti-metroo -n 500 --no-pager
   ```

4. **Try manual restart:**
   ```bash
   systemctl restart muti-metroo
   ```

5. **If still failing, start in foreground:**
   ```bash
   /usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml
   ```

### Network Partition Recovery

When agents become partitioned:

1. Verify network connectivity is restored
2. Agents will automatically reconnect via backoff
3. Monitor `peer_connections_total` metric
4. Check for handshake errors

If automatic recovery fails:
```bash
systemctl restart muti-metroo
```

### Security Incident Response

1. **Isolate affected agent:**
   ```bash
   systemctl stop muti-metroo
   ```

2. **Preserve evidence:**
   ```bash
   cp -r /var/log/journal /tmp/journal-backup
   tar -czvf evidence.tar.gz ./data/ ./config.yaml /tmp/journal-backup
   ```

3. **Rotate credentials:**
   - Generate new certificates
   - Update peer configurations
   - Restart all agents

4. **Review audit logs:**
   ```bash
   journalctl -u muti-metroo --since "2 hours ago" > incident.log
   ```

---

## Configuration Reference

### Environment Variables

Configuration values can be set via environment variables:

```yaml
agent:
  id: ${AGENT_ID:-auto}
  data_dir: ${DATA_DIR:-./data}

peers:
  - id: ${PEER_ID}
    address: ${PEER_ADDR}
    proxy_auth:
      username: ${PROXY_USER}
      password: ${PROXY_PASS}
```

### Default Values

| Setting | Default | Description |
|---------|---------|-------------|
| `agent.log_level` | info | Logging verbosity |
| `routing.advertise_interval` | 2m | Route advertisement frequency |
| `routing.route_ttl` | 5m | Route expiration time |
| `routing.max_hops` | 16 | Maximum route hop count |
| `connections.idle_threshold` | 5m | Stream idle timeout |
| `connections.timeout` | 90s | Connection timeout |
| `connections.reconnect.initial_delay` | 1s | Initial reconnect delay |
| `connections.reconnect.max_delay` | 60s | Maximum reconnect delay |
| `limits.max_streams_per_peer` | 1000 | Per-peer stream limit |
| `limits.max_streams_total` | 10000 | Global stream limit |
| `limits.buffer_size` | 256KB | Stream buffer size |
| `health.read_timeout` | 10s | Health endpoint timeout |
| `health.write_timeout` | 10s | Health endpoint timeout |

---

## Support and Resources

- **Repository:** https://github.com/coinstash/muti-metroo
- **Issues:** https://github.com/coinstash/muti-metroo/issues
- **Architecture Documentation:** See `Architecture.md`
- **Example Configurations:** See `configs/` directory
