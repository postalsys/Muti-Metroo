# Route Management API

HTTP endpoints for managing dynamic routes at runtime.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/routes/manage` | POST | Manage routes on local agent |
| `/agents/{agent-id}/routes/manage` | POST | Manage routes on remote agent |

These endpoints require `http.remote_api: true` in configuration.

---

## POST /routes/manage

Manage routes on the local agent.

### Request

Add a route:

```bash
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "network": "10.0.0.0/8", "metric": 0}'
```

Remove a route:

```bash
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "remove", "network": "10.0.0.0/8"}'
```

List all routes:

```bash
curl -X POST http://localhost:8080/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | Yes | Action to perform: `add`, `remove`, or `list` |
| `network` | string | For add/remove | CIDR network (e.g., `10.0.0.0/8`) |
| `metric` | integer | No | Route metric (default: 0, lower is preferred) |

### Response

**Add/Remove Success (200)**:

```json
{
  "status": "ok",
  "message": "route 10.0.0.0/8 added"
}
```

```json
{
  "status": "ok",
  "message": "route 10.0.0.0/8 removed"
}
```

**List Success (200)**:

```json
{
  "status": "ok",
  "routes": [
    {
      "network": "10.0.0.0/8",
      "metric": 0
    },
    {
      "network": "192.168.0.0/16",
      "metric": 5
    }
  ]
}
```

**Bad Request (400)**:

```json
{
  "error": "missing required field: network"
}
```

```json
{
  "error": "invalid CIDR network: 10.0.0.0"
}
```

```json
{
  "error": "cannot remove config route 10.0.0.0/8, only dynamic routes can be removed"
}
```

**Forbidden (403)**:

```
management key required but decryption unavailable
```

**Method Not Allowed (405)**:

```
Method Not Allowed
```

**Service Unavailable (503)**:

```
route management not configured
```

### Behavior

When a route is added:
1. The route is validated (CIDR format, no conflicts with config routes)
2. The route is added to the routing table
3. The route is immediately advertised to all connected peers

When a route is removed:
1. The route must be a dynamic route (not from config)
2. The route is removed from the routing table
3. The removal is advertised to all connected peers

Dynamic routes:
- Are ephemeral (lost on agent restart)
- Can be overridden by config routes with better metrics
- Cannot conflict with existing config routes

---

## POST /agents/\{agent-id\}/routes/manage

Manage routes on a remote agent.

### Request

Add a route on remote agent:

```bash
curl -X POST http://localhost:8080/agents/abc123def456/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "network": "172.16.0.0/12", "metric": 1}'
```

Remove a route on remote agent:

```bash
curl -X POST http://localhost:8080/agents/abc123def456/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "remove", "network": "172.16.0.0/12"}'
```

List routes on remote agent:

```bash
curl -X POST http://localhost:8080/agents/abc123def456/routes/manage \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}'
```

### Request Body

Same as `/routes/manage`.

### Response

Same response formats as `/routes/manage`.

The request is forwarded to the target agent via the mesh control channel. The response reflects the result from the remote agent.

### Error Responses

Additional error cases for remote route management:

**Agent Not Found (404)**:

```json
{
  "error": "agent not found or not reachable"
}
```

**Timeout (504)**:

```json
{
  "error": "request to remote agent timed out"
}
```

---

## Error Responses

All endpoints may return:

| Status | Description |
|--------|-------------|
| 400 | Invalid request body or parameters |
| 403 | Management key required but unavailable |
| 404 | Endpoint disabled (remote_api not enabled) or agent not found |
| 405 | Method not allowed (must be POST) |
| 503 | Route management not configured |
| 500 | Internal error (see error message) |
| 504 | Remote request timeout (remote endpoint only) |

:::note Management Key Protection
Route management endpoints are protected by the management key when configured. If `management.private_key` is set, the agent must be able to decrypt the management key to perform route management operations. This provides compartmentalization - only agents with the management private key can modify routes.
:::

:::warning Dynamic Routes are Ephemeral
Dynamic routes added via the API are lost when the agent restarts. For persistent routes, add them to the `exit.routes` section in the configuration file.
:::

---

## Examples

### Add Multiple Routes

```bash
#!/bin/bash
AGENT="http://localhost:8080"

ROUTES=(
  "10.0.0.0/8:0"
  "172.16.0.0/12:5"
  "192.168.0.0/16:10"
)

for ROUTE in "${ROUTES[@]}"; do
  NETWORK="${ROUTE%:*}"
  METRIC="${ROUTE#*:}"

  echo "Adding route $NETWORK with metric $METRIC..."
  curl -X POST "$AGENT/routes/manage" \
    -H "Content-Type: application/json" \
    -d "{\"action\": \"add\", \"network\": \"$NETWORK\", \"metric\": $METRIC}"
  echo
done
```

### Remove All Dynamic Routes

```bash
#!/bin/bash
AGENT="http://localhost:8080"

# Get all routes
ROUTES=$(curl -s -X POST "$AGENT/routes/manage" \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}' | jq -r '.routes[].network')

for NETWORK in $ROUTES; do
  echo "Removing route $NETWORK..."
  curl -X POST "$AGENT/routes/manage" \
    -H "Content-Type: application/json" \
    -d "{\"action\": \"remove\", \"network\": \"$NETWORK\"}"
  echo
done
```

### Centralized Route Management

Manage routes on all agents from a single control point:

```bash
#!/bin/bash
CONTROL_AGENT="http://localhost:8080"

# Get all agent IDs
AGENTS=$(curl -s "$CONTROL_AGENT/agents" | jq -r '.[].id')

# Add route to all agents
for AGENT_ID in $AGENTS; do
  echo "Adding route to agent $AGENT_ID..."
  curl -X POST "$CONTROL_AGENT/agents/$AGENT_ID/routes/manage" \
    -H "Content-Type: application/json" \
    -d '{"action": "add", "network": "10.100.0.0/16", "metric": 0}'
  echo
done
```

### Route Health Check

Monitor that critical routes exist:

```bash
#!/bin/bash
AGENT="http://localhost:8080"
CRITICAL_ROUTES=("10.0.0.0/8" "192.168.0.0/16")

# Get current routes
CURRENT=$(curl -s -X POST "$AGENT/routes/manage" \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}' | jq -r '.routes[].network')

for ROUTE in "${CRITICAL_ROUTES[@]}"; do
  if ! echo "$CURRENT" | grep -q "$ROUTE"; then
    echo "WARNING: Critical route $ROUTE is missing"
    # Add it back
    curl -X POST "$AGENT/routes/manage" \
      -H "Content-Type: application/json" \
      -d "{\"action\": \"add\", \"network\": \"$ROUTE\", \"metric\": 0}"
  else
    echo "OK: Route $ROUTE exists"
  fi
done
```

### Remote Route Synchronization

Synchronize routes from one agent to another:

```bash
#!/bin/bash
SOURCE_AGENT="http://localhost:8080"
TARGET_AGENT_ID="abc123def456"

# Get routes from source
ROUTES=$(curl -s -X POST "$SOURCE_AGENT/routes/manage" \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}' | jq -c '.routes[]')

# Add each route to target
echo "$ROUTES" | while read -r ROUTE; do
  NETWORK=$(echo "$ROUTE" | jq -r '.network')
  METRIC=$(echo "$ROUTE" | jq -r '.metric')

  echo "Copying route $NETWORK to target agent..."
  curl -X POST "$SOURCE_AGENT/agents/$TARGET_AGENT_ID/routes/manage" \
    -H "Content-Type: application/json" \
    -d "{\"action\": \"add\", \"network\": \"$NETWORK\", \"metric\": $METRIC}"
  echo
done
```

### Automated Failover Route

Add a backup route with higher metric when primary fails:

```bash
#!/bin/bash
AGENT="http://localhost:8080"
PRIMARY_ROUTE="10.0.0.0/8"
PRIMARY_METRIC=0
BACKUP_METRIC=100

# Check if primary route exists
CURRENT=$(curl -s -X POST "$AGENT/routes/manage" \
  -H "Content-Type: application/json" \
  -d '{"action": "list"}' | jq -r '.routes[] | select(.network=="'"$PRIMARY_ROUTE"'") | .metric')

if [ -z "$CURRENT" ]; then
  echo "Primary route missing, adding backup route with high metric..."
  curl -X POST "$AGENT/routes/manage" \
    -H "Content-Type: application/json" \
    -d "{\"action\": \"add\", \"network\": \"$PRIMARY_ROUTE\", \"metric\": $BACKUP_METRIC}"
else
  echo "Primary route exists with metric $CURRENT"
fi
```
