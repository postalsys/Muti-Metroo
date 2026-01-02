---
title: Agent Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole managing agents" style={{maxWidth: '180px'}} />
</div>

# Agent Endpoints

Remote agent status and management.

## GET /agents

List all known agents in the mesh.

**Response:**
```json
[
  {
    "id": "abc123def456789012345678901234ab",
    "short": "abc123de",
    "display_name": "Agent 1",
    "local": true
  },
  {
    "id": "def456789012345678901234567890cd",
    "short": "def45678",
    "local": false
  }
]
```

## GET /agents/\{agent-id\}

Get status from specific agent.

**Response:**
```json
{
  "id": "abc123...",
  "display_name": "Agent 1",
  "uptime": 3600,
  "peers": 3,
  "streams": 42,
  "routes": 5
}
```

## GET /agents/\{agent-id\}/routes

Get route table from specific agent.

**Response:**
```json
{
  "routes": [
    {
      "cidr": "10.0.0.0/8",
      "next_hop": "def456...",
      "metric": 1,
      "ttl": 300
    }
  ]
}
```

## GET /agents/\{agent-id\}/peers

Get peer list from specific agent.

**Response:**
```json
{
  "peers": [
    {
      "id": "def456...",
      "address": "192.168.1.20:4433",
      "transport": "quic",
      "connected": true
    }
  ]
}
```

## GET /agents/\{agent-id\}/metrics

Get Prometheus metrics from specific agent.

**Response:** Prometheus text format

```bash
curl http://localhost:8080/agents/abc123.../metrics
```

## POST /agents/\{agent-id\}/rpc

Execute RPC command on remote agent.

See [RPC Endpoints](rpc).

## POST /agents/\{agent-id\}/file/upload

Upload file to remote agent.

See [File Transfer Endpoints](file-transfer).

## POST /agents/\{agent-id\}/file/download

Download file from remote agent.

See [File Transfer Endpoints](file-transfer).
