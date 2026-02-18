# Forward Listener Management API

HTTP endpoints for managing dynamic forward listeners at runtime.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/forward/manage` | POST | Manage forward listeners on local agent |
| `/agents/{agent-id}/forward/manage` | POST | Manage forward listeners on remote agent |

These endpoints require `http.remote_api: true` in configuration.

---

## POST /forward/manage

Manage forward listeners on the local agent.

### Request

Add a listener:

```bash
curl -X POST http://localhost:8080/forward/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "key": "web-server", "address": ":9090"}'
```

Add a listener with connection limit:

```bash
curl -X POST http://localhost:8080/forward/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "key": "web-server", "address": ":9090", "max_connections": 100}'
```

Remove a listener:

```bash
curl -X POST http://localhost:8080/forward/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "remove", "key": "web-server"}'
```

List all listeners:

```bash
curl -X POST http://localhost:8080/forward/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | Yes | Action to perform: `add`, `remove`, or `list` |
| `key` | string | For add/remove | Routing key for the forward listener |
| `address` | string | For add | Listen address (e.g., `:9090`, `0.0.0.0:8080`) |
| `max_connections` | integer | No | Maximum concurrent connections (default: 0 = unlimited) |

### Response

**Add Success (200)**:

```json
{
  "status": "ok",
  "message": "forward listener \"web-server\" added on [::]:9090"
}
```

**Remove Success (200)**:

```json
{
  "status": "ok",
  "message": "forward listener \"web-server\" removed"
}
```

**List Success (200)**:

```json
{
  "status": "ok",
  "listeners": [
    {
      "key": "web-server",
      "address": "[::]:9090",
      "max_connections": 0,
      "dynamic": false
    },
    {
      "key": "api-server",
      "address": "[::]:8081",
      "max_connections": 100,
      "dynamic": true
    }
  ]
}
```

**Bad Request (400)**:

```json
{
  "error": "key is required"
}
```

```json
{
  "error": "listener \"web-server\" is a config listener and cannot be removed"
}
```

```json
{
  "error": "listener \"web-server\" not found"
}
```

**Forbidden (403)**:

```
forward management restricted: management key decryption unavailable
```

### Behavior

When a listener is added:
1. The key and address are validated
2. If the key belongs to a config listener, the request is rejected
3. If a dynamic listener with the same key exists, it is stopped and replaced
4. The new listener is started on the specified address
5. Node info is immediately re-advertised to peers

When a listener is removed:
1. The key must belong to a dynamic listener (not from config)
2. The listener is stopped and removed
3. Node info is immediately re-advertised to peers

When listing:
- Both config and dynamic listeners are returned
- The `dynamic` field distinguishes runtime listeners from config-file listeners

Dynamic listeners:
- Are ephemeral (lost on agent restart)
- Can be replaced by adding the same key again
- Config-file listeners are protected from modification

---

## POST /agents/\{agent-id\}/forward/manage

Manage forward listeners on a remote agent.

### Request

```bash
curl -X POST http://localhost:8080/agents/abc123def456/forward/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "key": "web-server", "address": ":9090"}'
```

### Request Body

Same as `/forward/manage`.

### Response

Same response formats as `/forward/manage`.

The request is forwarded to the target agent via the mesh control channel. The response reflects the result from the remote agent.

---

## Error Responses

All endpoints may return:

| Status | Description |
|--------|-------------|
| 400 | Invalid request body or parameters |
| 403 | Management key required but unavailable |
| 404 | Endpoint disabled (remote_api not enabled) or agent not found |
| 405 | Method not allowed (must be POST) |
| 503 | Forward management not configured |

:::note Management Key Protection
Forward listener management endpoints follow the same management key restrictions as route management. Agents with only `management.public_key` (field agents) cannot manage listeners. Agents with both keys (operator nodes) can manage listeners freely.
:::

:::warning Dynamic Listeners are Ephemeral
Dynamic listeners added via the API are lost when the agent restarts. For persistent listeners, add them to the `forward.listeners` section in the configuration file.
:::
