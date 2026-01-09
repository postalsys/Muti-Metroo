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
  dashboard: true          # Web dashboard
  remote_api: true         # Remote agent APIs
```

## Endpoint Control

| Setting | Endpoints | Default |
|---------|-----------|---------|
| `minimal: true` | Only `/health`, `/healthz`, `/ready` | false |
| `pprof: false` | Disable `/debug/pprof/*` | false |
| `dashboard: false` | Disable `/ui/*`, `/api/*` | true |
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

## Dashboard Endpoints

### GET /ui/

Web dashboard with metro map visualization. Open in browser:

```bash
open http://localhost:8080/ui/
```

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
curl http://localhost:8080/agents/abc123def456/status | jq
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

### OPSEC Configuration

For maximum stealth:

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
