---
title: Health Endpoints
---

# Health Endpoints

Health check and readiness endpoints.

## GET /health

Basic health check.

**Response:**
```
OK
```

**Status:** 200 OK

## GET /healthz

Detailed health with statistics.

**Response:**
```json
{
  "status": "healthy",
  "agent_id": "abc123...",
  "peers": 3,
  "streams": 42,
  "routes": 5,
  "uptime": 3600
}
```

## GET /ready

Kubernetes readiness probe.

**Response:**
```
Ready
```

Returns 200 if agent is ready to accept traffic.

## Examples

```bash
# Basic health check
curl http://localhost:8080/health

# Detailed health
curl http://localhost:8080/healthz

# Readiness
curl http://localhost:8080/ready
```
