---
title: Web Dashboard
sidebar_position: 8
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting dashboard" style={{maxWidth: '180px'}} />
</div>

# Web Dashboard

See your entire mesh at a glance. The built-in dashboard shows all agents, their connections, and active routes - updated in real time as your network changes.

```bash
# Open the dashboard
open http://localhost:8080/ui/
```

## What You Can See

- **All agents** in your mesh and how they connect
- **Active routes** and which exit handles each destination
- **Connection status** - know instantly when a link goes down
- **Agent details** - click any node to see system info, peers, and routes

<div style={{textAlign: 'center', margin: '2rem 0'}}>
  <img src="/img/dashboard-screenshot.png" alt="Muti Metroo Dashboard" style={{maxWidth: '100%', borderRadius: '8px', boxShadow: '0 4px 12px rgba(0,0,0,0.15)'}} />
  <p style={{marginTop: '0.5rem', fontStyle: 'italic', color: '#666'}}>Dashboard showing a 4-agent mesh topology</p>
</div>

### Node Information

- **Agent details**: ID, display name, IP address
- **System info**: OS, architecture, uptime
- **Connectivity**: Connected peers and routes
- **Metrics**: Active streams, data transfer

### Dashboard Overview

- **Agent info**: Local agent details
- **Stats**: Peer count, stream count, route count
- **Peers**: Connected peer list
- **Routes**: Advertised and learned routes
- **Port Forward Routes**: Ingress-exit pairs for port forwarding

### Port Forward Routes Table

When port forwarding is configured in your mesh, the dashboard displays all active ingress-exit pairs:

| Column | Description |
|--------|-------------|
| **Key** | The routing key that links ingress listeners to exit endpoints |
| **Ingress** | Agent running the listener (where clients connect) |
| **Listener** | The listen address on the ingress agent |
| **Exit** | Agent running the endpoint (where connections are forwarded) |
| **Target** | The target service address on the exit agent |
| **Hops** | Number of hops from ingress to exit agent |

This view shows all possible routes through the mesh. If multiple ingress agents have listeners for the same key, or multiple exit agents have endpoints, all combinations are displayed.

## API Endpoints

### Dashboard Data

`GET /api/dashboard`

Returns complete dashboard overview:
```json
{
  "agent": {
    "id": "abc123",
    "display_name": "My Agent",
    "uptime": 3600
  },
  "stats": {
    "peers": 3,
    "streams": 42,
    "routes": 5
  },
  "peers": [...],
  "routes": [...]
}
```

### Topology Data

`GET /api/topology`

Returns metro map data:
```json
{
  "agents": [
    {
      "id": "abc123",
      "name": "Agent 1",
      "ip": "192.168.1.10"
    }
  ],
  "connections": [
    {
      "source": "abc123",
      "target": "def456",
      "transport": "quic"
    }
  ]
}
```

### Node Details

`GET /api/nodes`

Returns detailed node information for all known agents.

:::tip Configuration
See [HTTP Configuration](/configuration/http) for dashboard access options including binding address and minimal mode.
:::

## Command Line Access

Query dashboard data without a browser:

```bash
# Get dashboard overview
curl http://localhost:8080/api/dashboard | jq

# Get topology for mesh visualization
curl http://localhost:8080/api/topology | jq

# Get detailed node information
curl http://localhost:8080/api/nodes | jq

# Quick health check
curl http://localhost:8080/healthz | jq '{peers: .peer_count, streams: .stream_count, routes: .routes | length}'
```

## Troubleshooting

### Dashboard Not Loading

```
Connection refused on http://localhost:8080/ui/
```

**Causes:**
- HTTP server not enabled in configuration
- Agent not running
- Wrong port number

**Solutions:**
```bash
# Check if HTTP server is running
curl http://localhost:8080/health

# Verify configuration has http.enabled: true
```

### No Agents Visible

```
Dashboard shows only local agent, no peers
```

**Causes:**
- No peers connected yet
- Peers haven't advertised their routes
- Network connectivity issues

**Solutions:**
```bash
# Check peer connections
curl http://localhost:8080/healthz | jq '.peers'

# Trigger route advertisement
curl -X POST http://localhost:8080/routes/advertise
```

### Dashboard Returns 404

```
HTTP 404 on /ui/ endpoint
```

**Cause:** Dashboard disabled with `http.minimal: true` or `http.dashboard: false`.

**Solution:** Enable dashboard in configuration or use the API endpoints directly.

## Related

- [API - Dashboard](/api/dashboard) - Dashboard API reference
- [API - Health](/api/health) - Health endpoints
- [Deployment - Docker](/deployment/docker) - Running with Docker
