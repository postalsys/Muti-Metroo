---
title: HTTP API Overview
---

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
| [Health](health) | Health checks and readiness probes |
| [Metrics](metrics) | Prometheus metrics (local and remote) |
| [Agents](agents) | Remote agent status and management |
| [Routes](routes) | Route management and triggers |
| [RPC](rpc) | Remote command execution |
| [File Transfer](file-transfer) | File upload/download |
| [Dashboard](dashboard) | Web dashboard and topology data |

## Authentication

Most endpoints require no authentication. RPC and file transfer endpoints require password authentication when configured.

## Response Formats

- **JSON**: Most endpoints return JSON
- **Plain text**: Health checks return plain text
- **Binary**: File downloads return binary data
- **Prometheus**: Metrics endpoint returns Prometheus format

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
