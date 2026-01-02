---
title: Dashboard Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting dashboard" style={{maxWidth: '180px'}} />
</div>

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
    "id": "abc123def456789012345678901234ab",
    "short_id": "abc123de",
    "display_name": "My Agent",
    "is_local": true,
    "is_connected": true
  },
  "stats": {
    "peer_count": 3,
    "stream_count": 42,
    "route_count": 5,
    "socks5_running": true,
    "exit_handler_running": false
  },
  "peers": [
    {
      "id": "def456789012345678901234567890cd",
      "short_id": "def45678",
      "display_name": "Peer 1",
      "state": "connected",
      "rtt_ms": 15,
      "is_dialer": true
    }
  ],
  "routes": [
    {
      "network": "10.0.0.0/8",
      "origin": "Exit Node",
      "origin_id": "exit1234",
      "hop_count": 2,
      "path_display": ["My Agent", "Transit", "Exit Node"]
    }
  ]
}
```

## GET /api/topology

Metro map topology data for visualization.

**Response:**
```json
{
  "local_agent": {
    "id": "abc123def456789012345678901234ab",
    "short_id": "abc123de",
    "display_name": "My Agent",
    "is_local": true,
    "is_connected": true,
    "hostname": "server1.example.com",
    "os": "linux",
    "arch": "amd64",
    "version": "1.0.7",
    "uptime_hours": 24.5,
    "ip_addresses": ["192.168.1.10", "10.0.0.1"]
  },
  "agents": [
    {
      "id": "abc123def456789012345678901234ab",
      "short_id": "abc123de",
      "display_name": "My Agent",
      "is_local": true,
      "is_connected": true
    },
    {
      "id": "def456789012345678901234567890cd",
      "short_id": "def45678",
      "display_name": "Peer 1",
      "is_local": false,
      "is_connected": true,
      "hostname": "peer1.example.com",
      "os": "linux",
      "arch": "amd64"
    }
  ],
  "connections": [
    {
      "from_agent": "abc123de",
      "to_agent": "def45678",
      "is_direct": true,
      "rtt_ms": 15,
      "transport": "quic"
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
      "id": "abc123def456789012345678901234ab",
      "short_id": "abc123de",
      "display_name": "Agent 1",
      "is_local": true,
      "is_connected": true,
      "hostname": "agent1.example.com",
      "os": "linux",
      "arch": "amd64",
      "version": "1.0.7",
      "uptime_hours": 24.5,
      "ip_addresses": ["192.168.1.10", "10.0.0.1"]
    },
    {
      "id": "def456789012345678901234567890cd",
      "short_id": "def45678",
      "display_name": "Agent 2",
      "is_local": false,
      "is_connected": true,
      "hostname": "agent2.example.com",
      "os": "darwin",
      "arch": "arm64",
      "version": "1.0.7",
      "uptime_hours": 12.0,
      "ip_addresses": ["192.168.1.20"]
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
