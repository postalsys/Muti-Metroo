---
title: Agent Configuration
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole configuring agent" style={{maxWidth: '180px'}} />
</div>

# Agent Configuration

The `agent` section configures agent identity and logging.

## Configuration

```yaml
agent:
  # Agent ID
  id: "auto"                    # "auto" or 32-character hex string

  # Human-readable name
  display_name: "My Agent"      # Shown in dashboard and logs

  # Data directory
  data_dir: "./data"            # For agent_id file and control socket

  # Logging
  log_level: "info"             # debug, info, warn, error
  log_format: "text"            # text, json
```

## Agent ID

Every agent has a unique 128-bit identifier.

### Auto-generation

```yaml
agent:
  id: "auto"
  data_dir: "./data"
```

On first run:
1. Check if `./data/agent_id` exists
2. If not, generate new random ID
3. Save to `./data/agent_id`
4. Use for all peer communications

### Manual ID

Specify a specific ID:

```yaml
agent:
  id: "a1b2c3d4e5f6789012345678901234ab"
```

Requirements:
- Exactly 32 hexadecimal characters
- Unique across your mesh

### Viewing Agent ID

```bash
# From file
cat ./data/agent_id

# From running agent
curl http://localhost:8080/healthz | jq '.agent_id'

# From logs
INFO  Agent ID: a1b2c3d4e5f6789012345678901234ab
```

## Display Name

Human-readable name for the agent:

```yaml
agent:
  display_name: "Office Gateway"
```

Used in:
- Web dashboard
- Node info advertisements
- Logs and metrics

Supports Unicode:

```yaml
agent:
  display_name: "Tallinn Gateway"
```

If not set, the agent ID is used for display.

## Data Directory

Where agent persists state:

```yaml
agent:
  data_dir: "./data"
```

Contents:
- `agent_id` - Agent identity file
- `control.sock` - Unix socket for CLI (if enabled)

### Permissions

```bash
# Create with appropriate permissions
mkdir -p ./data
chmod 700 ./data
```

### Shared Storage Warning

Do not share `data_dir` between multiple agents - each agent needs its own identity.

## Logging

### Log Level

```yaml
agent:
  log_level: "info"    # debug, info, warn, error
```

| Level | Description |
|-------|-------------|
| `debug` | Verbose debugging (frames, routing) |
| `info` | Normal operation (connections, streams) |
| `warn` | Warnings (reconnections, timeouts) |
| `error` | Errors only |

### Log Format

```yaml
agent:
  log_format: "text"    # text, json
```

**Text format** (human-readable):

```
2025-01-15T10:30:45Z INFO  Starting Muti Metroo agent
2025-01-15T10:30:45Z INFO  Agent ID: a1b2c3d4...
2025-01-15T10:30:45Z INFO  SOCKS5 server started on 127.0.0.1:1080
```

**JSON format** (machine-readable):

```json
{"time":"2025-01-15T10:30:45Z","level":"INFO","msg":"Starting Muti Metroo agent"}
{"time":"2025-01-15T10:30:45Z","level":"INFO","msg":"Agent ID","agent_id":"a1b2c3d4..."}
```

### Log Destination

Logs go to stderr by default. Redirect as needed:

```bash
# To file
./build/muti-metroo run -c config.yaml 2> agent.log

# With rotation (using logrotate)
./build/muti-metroo run -c config.yaml 2>> /var/log/muti-metroo/agent.log
```

### Runtime Log Level

Log level can be set at runtime via flag:

```bash
./build/muti-metroo run -c config.yaml --log-level debug
```

This overrides the config file setting.

## Environment Variables

Use environment variables for deployment flexibility:

```yaml
agent:
  id: "${AGENT_ID:-auto}"
  display_name: "${AGENT_NAME:-}"
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"
  log_format: "${LOG_FORMAT:-text}"
```

## Examples

### Development

```yaml
agent:
  id: "auto"
  display_name: "Dev Agent"
  data_dir: "./data"
  log_level: "debug"
  log_format: "text"
```

### Production

```yaml
agent:
  id: "auto"
  display_name: "${HOSTNAME}"
  data_dir: "/var/lib/muti-metroo"
  log_level: "info"
  log_format: "json"
```

### Docker

```yaml
agent:
  id: "auto"
  display_name: "${AGENT_NAME:-container}"
  data_dir: "/app/data"
  log_level: "${LOG_LEVEL:-info}"
  log_format: "json"
```

## Related

- [Getting Started](../getting-started/quick-start) - Initial setup
- [Listeners](listeners) - Transport configuration
- [Deployment](../deployment/scenarios) - Production deployment
