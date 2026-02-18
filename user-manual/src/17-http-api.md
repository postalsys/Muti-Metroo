# HTTP API

The HTTP API provides endpoints for health monitoring, status, and management.

## Configuration

```yaml
http:
  enabled: true
  address: ":8080"
  read_timeout: 10s
  write_timeout: 10s
  minimal: false           # Only health endpoints
  pprof: false             # Debug endpoints
  dashboard: true          # Dashboard API endpoints
  remote_api: true         # Remote agent APIs
```

## Endpoint Control

| Setting | Endpoints | Default |
|---------|-----------|---------|
| `minimal: true` | Only `/health`, `/healthz`, `/ready` | false |
| `pprof: false` | Disable `/debug/pprof/*` | false |
| `dashboard: false` | Disable `/api/*` | true |
| `remote_api: false` | Disable `/agents/*` | true |

Disabled endpoints return HTTP 404.

## Health Endpoints

### GET /health

Basic health check, returns "OK":

```bash
curl http://localhost:8080/health
# Output: OK
```

### GET /healthz

Detailed health with JSON stats:

```bash
curl http://localhost:8080/healthz | jq
```

Response:

```json
{
  "status": "healthy",
  "agent_id": "a1b2c3d4...",
  "display_name": "My Agent",
  "running": true,
  "peer_count": 2,
  "stream_count": 5,
  "route_count": 3,
  "uptime_seconds": 3600
}
```

### GET /ready

Readiness probe for orchestration systems:

```bash
curl http://localhost:8080/ready
```

## Dashboard API Endpoints

### GET /api/topology

Topology data for the metro map:

```bash
curl http://localhost:8080/api/topology | jq
```

### GET /api/dashboard

Dashboard overview with agent info, stats, peers, and routes:

```bash
curl http://localhost:8080/api/dashboard | jq
```

### GET/POST /api/mesh-test

Test connectivity to all known agents in the mesh. GET returns cached results
(30-second TTL), POST forces a fresh test:

```bash
# Fresh test
curl -X POST http://localhost:8080/api/mesh-test | jq

# Cached results
curl http://localhost:8080/api/mesh-test | jq
```

Response:

```json
{
  "local_agent": "abc123de",
  "test_time": "2026-01-27T10:30:00Z",
  "duration_ms": 1200,
  "total_count": 5,
  "reachable_count": 4,
  "results": [
    {
      "agent_id": "abc123def456...",
      "short_id": "abc123de",
      "display_name": "gateway-1",
      "is_local": true,
      "reachable": true,
      "response_time_ms": 0
    }
  ]
}
```

Also available via CLI: `muti-metroo mesh-test`

### GET /api/nodes

Detailed node info for all known agents:

```bash
curl http://localhost:8080/api/nodes | jq
```

## Remote Agent Endpoints

Query other agents through the mesh:

### GET /agents

List all known agents:

```bash
curl http://localhost:8080/agents | jq
```

### GET /agents/{agent-id}

Get status from a specific agent:

```bash
curl http://localhost:8080/agents/abc123def456 | jq
```

### GET /agents/{agent-id}/routes

Get route table from a specific agent:

```bash
curl http://localhost:8080/agents/abc123def456/routes | jq
```

### GET /agents/{agent-id}/peers

Get peer list from a specific agent:

```bash
curl http://localhost:8080/agents/abc123def456/peers | jq
```

### POST /agents/{agent-id}/file/browse

Browse the filesystem on a remote agent (directory listing, stat, roots):

```bash
curl -X POST http://localhost:8080/agents/abc123/file/browse \
  -H "Content-Type: application/json" \
  -d '{"action":"list","path":"/tmp"}'
```

## Management Endpoints

### POST /routes/advertise

Trigger immediate route advertisement:

```bash
curl -X POST http://localhost:8080/routes/advertise
```

Response:

```json
{
  "status": "triggered",
  "message": "route advertisement triggered"
}
```

### POST /routes/manage

Add, remove, or list dynamic CIDR exit routes:

```bash
# Add a route
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action":"add","network":"10.0.0.0/8"}'

# Add with custom metric
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action":"add","network":"192.168.0.0/16","metric":5}'

# List routes
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action":"list"}'

# Remove a route
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action":"remove","network":"10.0.0.0/8"}'
```

Response:

```json
{
  "status": "success",
  "message": "route added",
  "routes": [
    {
      "network": "10.0.0.0/8",
      "metric": 1,
      "source": "dynamic"
    }
  ]
}
```

### POST /agents/{agent-id}/routes/manage

Manage routes on a remote agent:

```bash
curl -X POST http://localhost:8080/agents/abc123def456/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action":"add","network":"10.0.0.0/8"}'
```

## Sleep Mode Endpoints

Control mesh hibernation via HTTP.

### POST /sleep

Put the mesh into sleep mode:

```bash
curl -X POST http://localhost:8080/sleep
```

Response:

```json
{
  "status": "triggered",
  "message": "sleep command triggered"
}
```

### POST /wake

Wake the mesh from sleep mode:

```bash
curl -X POST http://localhost:8080/wake
```

Response:

```json
{
  "status": "triggered",
  "message": "wake command triggered"
}
```

### GET /sleep/status

Get current sleep status:

```bash
curl http://localhost:8080/sleep/status | jq
```

Response:

```json
{
  "state": "SLEEPING",
  "enabled": true,
  "sleep_start_time": "2026-01-19T10:30:00Z",
  "last_poll_time": "2026-01-19T10:35:00Z",
  "next_poll_time": "2026-01-19T10:40:00Z",
  "queued_peers": 2
}
```

States: `AWAKE`, `SLEEPING`, `POLLING`

## CLI Commands

Use CLI for convenient access:

```bash
# Agent status
muti-metroo status -a localhost:8080

# Peer list
muti-metroo peers -a localhost:8080

# Route table
muti-metroo routes -a localhost:8080
```

## Security Recommendations

### Production Configuration

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"  # Localhost only
  minimal: false
  pprof: false               # Never in production
  dashboard: true
  remote_api: true
```

### Minimal Configuration

For production deployments with reduced exposure:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true              # Only health endpoints
```

### Exposed Agent

If HTTP API must be exposed:

1. Use a firewall to limit access
2. Consider reverse proxy with authentication
3. Disable unnecessary endpoints:

```yaml
http:
  enabled: true
  address: "0.0.0.0:8080"
  pprof: false
  dashboard: false
  remote_api: false
```

## Examples

### Quick Health Check Script

```bash
#!/bin/bash
for agent in "localhost:8080" "remote1:8080" "remote2:8082"; do
  echo "=== $agent ==="
  curl -s "http://$agent/healthz" | jq '{running, peers: .peer_count, routes: .route_count}'
done
```

### Monitoring Integration

Prometheus-style health check:

```bash
# Returns 200 if healthy, 503 if not
curl -f http://localhost:8080/ready
```

### Verify Topology

```bash
# Count known nodes
curl -s http://localhost:8080/api/nodes | jq '.nodes | length'

# List connected peers
curl -s http://localhost:8080/api/dashboard | jq '.peers[].display_name'

# Check route paths
curl -s http://localhost:8080/api/dashboard | jq '.routes[] | {network, hops: .hop_count}'
```
