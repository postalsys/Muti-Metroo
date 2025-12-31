---
title: Dashboard Endpoints
---

# Dashboard Endpoints

Web dashboard and topology data APIs.

## GET /ui/

Embedded web dashboard interface.

Access via browser: `http://localhost:8080/ui/`

Features:
- Metro map visualization
- Agent information
- Live topology updates
- Node details

## GET /api/dashboard

Dashboard overview data.

**Response:**
```json
{
  "agent": {
    "id": "abc123...",
    "display_name": "My Agent",
    "uptime": 3600
  },
  "stats": {
    "peers": 3,
    "streams": 42,
    "routes": 5
  },
  "peers": [
    {
      "id": "def456...",
      "address": "192.168.1.20:4433",
      "transport": "quic"
    }
  ],
  "routes": [
    {
      "cidr": "10.0.0.0/8",
      "metric": 1
    }
  ]
}
```

## GET /api/topology

Metro map topology data.

**Response:**
```json
{
  "agents": [
    {
      "id": "abc123...",
      "name": "Agent 1",
      "ip": "192.168.1.10",
      "os": "linux",
      "arch": "amd64"
    }
  ],
  "connections": [
    {
      "source": "abc123...",
      "target": "def456...",
      "transport": "quic",
      "state": "connected"
    }
  ]
}
```

## GET /api/nodes

Detailed node information for all known agents.

**Response:**
```json
{
  "nodes": [
    {
      "id": "abc123...",
      "display_name": "Agent 1",
      "hostname": "agent1.example.com",
      "ip": "192.168.1.10",
      "os": "linux",
      "arch": "amd64",
      "uptime": 3600,
      "last_seen": "2025-01-01T00:00:00Z"
    }
  ]
}
```

## Examples

```bash
# Get dashboard data
curl http://localhost:8080/api/dashboard

# Get topology
curl http://localhost:8080/api/topology

# Get node details
curl http://localhost:8080/api/nodes
```

See [Web Dashboard Feature](../features/web-dashboard) for more information.
