---
title: HTTP API Overview
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole connecting APIs" style={{maxWidth: '180px'}} />
</div>

# HTTP API Reference

Complete HTTP API reference for Muti Metroo.

## Base URL

```
http://localhost:8080
```

Configure via:

```yaml
http:
  enabled: true
  address: ":8080"
```

## Endpoint Categories

| Category | Purpose |
|----------|---------|
| [Health](/api/health) | Health checks and readiness probes |
| [Agents](/api/agents) | Remote agent status and management |
| [Routes](/api/routes) | Route management and triggers |
| [Shell](/api/shell) | Remote shell access (interactive and streaming) |
| [File Transfer](/api/file-transfer) | File upload/download |
| [Dashboard](/api/dashboard) | Web dashboard and topology data |

## Authentication

Most endpoints require no authentication. Shell and file transfer endpoints require password authentication when configured.

## Response Formats

- **JSON**: Most endpoints return JSON
- **Plain text**: Health checks return plain text
- **Binary**: File downloads return binary data

## Error Responses

```json
{
  "error": "error message",
  "code": "ERROR_CODE"
}
```

Common HTTP status codes:
- `200 OK`: Success
- `400 Bad Request`: Invalid request
- `401 Unauthorized`: Authentication failed
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error
