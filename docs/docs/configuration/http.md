---
title: HTTP API
sidebar_position: 11
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole configuring HTTP API" style={{maxWidth: '180px'}} />
</div>

# HTTP API Configuration

Enable health checks, the web dashboard, and remote agent APIs. The HTTP server is your window into the mesh - use it for monitoring, visualization, and distributed operations.

**Most common settings:**
```yaml
http:
  enabled: true
  address: ":8080"
```

## Configuration

```yaml
http:
  enabled: true           # Enable HTTP API server
  address: ":8080"        # Bind address (host:port)
  read_timeout: 10s       # Request read timeout
  write_timeout: 10s      # Response write timeout

  # Endpoint controls
  minimal: false          # When true, only health endpoints enabled
  pprof: false            # /debug/pprof/* profiling endpoints
  dashboard: true         # /ui/* web dashboard
  remote_api: true        # /agents/* distributed APIs
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the HTTP API server |
| `address` | string | `:8080` | Bind address (`:8080` or `127.0.0.1:8080`) |
| `read_timeout` | duration | `10s` | Maximum time to read request |
| `write_timeout` | duration | `10s` | Maximum time to write response |
| `minimal` | bool | `false` | Only enable health endpoints |
| `pprof` | bool | `false` | Enable Go profiling endpoints |
| `dashboard` | bool | `true` | Enable web dashboard |
| `remote_api` | bool | `true` | Enable distributed mesh APIs |

## Endpoints

### Always Available

These endpoints are always enabled when `http.enabled: true`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Simple health check, returns "OK" |
| `/healthz` | GET | Detailed health with JSON stats |
| `/ready` | GET | Readiness probe for load balancers |
| `/routes/advertise` | POST | Trigger immediate route advertisement |

### Dashboard Endpoints

Enabled when `dashboard: true`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ui/` | GET | Web dashboard with metro map |
| `/api/topology` | GET | Topology data for visualization |
| `/api/dashboard` | GET | Dashboard overview (stats, peers, routes) |
| `/api/nodes` | GET | Detailed node info for all agents |

### Remote API Endpoints

Enabled when `remote_api: true`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/agents` | GET | List all known agents |
| `/agents/{id}` | GET | Get status from specific agent |
| `/agents/{id}/routes` | GET | Get route table from agent |
| `/agents/{id}/peers` | GET | Get peer list from agent |
| `/agents/{id}/shell` | WebSocket | Remote shell access |
| `/agents/{id}/icmp` | WebSocket | ICMP ping sessions |
| `/agents/{id}/file/upload` | POST | Upload file to agent |
| `/agents/{id}/file/download` | POST | Download file from agent |

### Profiling Endpoints

Enabled when `pprof: true`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/debug/pprof/` | GET | Profiling index |
| `/debug/pprof/profile` | GET | CPU profile |
| `/debug/pprof/heap` | GET | Heap profile |
| `/debug/pprof/goroutine` | GET | Goroutine stacks |

:::warning Production Security
Disable `pprof` in production - profiling endpoints can leak sensitive information and consume significant resources.
:::

## Minimal Mode

For maximum OPSEC, enable only health endpoints:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"  # Localhost only
  minimal: true              # Only /health, /healthz, /ready
```

When `minimal: true`, all endpoint flags (`pprof`, `dashboard`, `remote_api`) are ignored and those endpoints return HTTP 404.

## Bind Address

### All Interfaces

```yaml
http:
  address: ":8080"  # Listen on all interfaces
```

### Localhost Only

```yaml
http:
  address: "127.0.0.1:8080"  # Local access only
```

### Specific Interface

```yaml
http:
  address: "192.168.1.10:8080"  # Specific IP only
```

## Security Considerations

| Configuration | Access | Use Case |
|---------------|--------|----------|
| `address: "127.0.0.1:8080"` | Local only | Development, single-user |
| `address: ":8080"` + firewall | Controlled | Production with network controls |
| `minimal: true` | Health only | High-security field deployments |
| `pprof: true` | Profiling | Debugging only, never production |

### Recommendations

1. **Bind to localhost** in production unless remote access is required
2. **Disable pprof** in production deployments
3. **Use minimal mode** for field agents that don't need dashboard
4. **Firewall the port** if binding to all interfaces

## Examples

### Development

```yaml
http:
  enabled: true
  address: ":8080"
  pprof: true       # Enable profiling for debugging
  dashboard: true
  remote_api: true
```

### Production

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  pprof: false      # Disable profiling
  dashboard: true
  remote_api: true
```

### Field Agent (OPSEC)

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true     # Health endpoints only
```

### Monitoring Integration

```yaml
http:
  enabled: true
  address: ":8080"
  minimal: false
  dashboard: false  # No web UI needed
  remote_api: false # No distributed APIs
  # Only /health, /healthz, /ready for load balancer probes
```

## Environment Variables

```yaml
http:
  enabled: ${HTTP_ENABLED:-true}
  address: "${HTTP_ADDRESS:-:8080}"
  minimal: ${HTTP_MINIMAL:-false}
```

## Related

- [HTTP API Reference](/api/overview) - Complete API documentation
- [Troubleshooting](/troubleshooting/common-issues) - Connection issues
- [Deployment](/deployment/scenarios) - Production deployment patterns
