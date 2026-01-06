---
title: Web Dashboard
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting dashboard" style={{maxWidth: '180px'}} />
</div>

# Web Dashboard

Embedded web interface with metro map visualization of mesh topology.

## Access

```bash
# Enable HTTP server in config
http:
  enabled: true
  address: ":8080"

# Access dashboard
open http://localhost:8080/ui/
```

## Features

### Metro Map Visualization

- **Visual topology**: See all agents and connections as a metro map
- **Real-time updates**: Live status of connections and routes
- **Interactive**: Click nodes for details

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

## Configuration

Dashboard is automatically available when HTTP server is enabled:

```yaml
http:
  enabled: true
  address: ":8080"
```

No additional configuration required.

## Related

- [API - Dashboard](/api/dashboard) - Dashboard API reference
- [API - Health](/api/health) - Health endpoints
- [Deployment - Docker](/deployment/docker) - Running with Docker
