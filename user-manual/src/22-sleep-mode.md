# Sleep Mode

Sleep mode enables mesh hibernation - agents close all peer connections and enter a low-profile dormant state, periodically polling for queued messages.

## Overview

When sleep mode is enabled, agents transition between three states:

- **AWAKE**: Normal operation with all connections active
- **SLEEPING**: Dormant state with all peer connections closed
- **POLLING**: Briefly reconnected to receive queued messages

Sleep and wake commands flood through the mesh, allowing you to hibernate or wake the entire network from any connected agent.

## Use Cases

- **Covert deployments**: Keep agents dormant until explicitly needed
- **Resource conservation**: Reduce network and memory footprint during inactive periods
- **Traffic pattern reduction**: Minimize network connections when not in use
- **Scheduled operations**: Sleep during off-hours, wake for maintenance windows

## Configuration

Enable sleep mode in your agent configuration:

```yaml
sleep:
  enabled: true
  poll_interval: 5m          # How often to poll while sleeping
  poll_interval_jitter: 0.3  # +/- 30% timing variation
  poll_duration: 30s         # How long to stay connected per poll
  persist_state: true        # Survive agent restarts
  max_queued_messages: 1000  # Queue limit per sleeping peer
  auto_sleep_on_start: false # Start in sleep mode
```

### Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `false` | Enable sleep mode |
| `poll_interval` | `5m` | Base interval between poll cycles |
| `poll_interval_jitter` | `0.3` | Timing jitter fraction (0.0-1.0) |
| `poll_duration` | `30s` | How long to stay connected per poll |
| `persist_state` | `true` | Save sleep state to disk |
| `max_queued_messages` | `1000` | Queue limit per sleeping peer |
| `auto_sleep_on_start` | `false` | Start in sleep mode automatically |

## CLI Commands

### Put Mesh to Sleep

```bash
muti-metroo sleep

# Via a specific agent
muti-metroo sleep -a 192.168.1.10:8080
```

### Wake the Mesh

```bash
muti-metroo wake

# Via a specific agent
muti-metroo wake -a 192.168.1.10:8080
```

### Check Status

```bash
muti-metroo sleep-status

# JSON output
muti-metroo sleep-status --json
```

Example output:

```
Sleep Mode Status
=================
State:          SLEEPING
Enabled:        true
Sleep Started:  2026-01-19T10:30:00Z
Last Poll:      2026-01-19T10:35:00Z
Next Poll:      2026-01-19T10:40:00Z
Queued Peers:   2
```

## How It Works

### Entering Sleep Mode

When sleep is triggered:

1. A SLEEP_COMMAND floods to all connected peers
2. Each agent closes all peer connections
3. SOCKS5 and listener services stop
4. A poll timer is scheduled with jittered timing

### Poll Cycles

While sleeping, agents periodically wake briefly to:

1. Reconnect to configured peers
2. Receive queued route advertisements and node info
3. Check for wake commands
4. Disconnect and return to sleep

The poll interval includes configurable jitter to prevent predictable connection patterns.

### Waking Up

When wake is triggered:

1. The agent reconnects to all configured peers
2. SOCKS5 and listener services restart
3. A WAKE_COMMAND floods to the mesh
4. Normal operation resumes

## State Persistence

When `persist_state` is enabled, sleep state survives agent restarts:

- An agent sleeping when stopped resumes sleeping when restarted
- Poll timing and command sequence are preserved
- No manual intervention needed after system reboots

## Auto-Sleep on Start

For covert deployments, enable `auto_sleep_on_start`:

```yaml
sleep:
  enabled: true
  auto_sleep_on_start: true
```

The agent will:

1. Start and initialize normally
2. Wait 5 seconds for connections to establish
3. Automatically enter sleep mode
4. Remain dormant until explicitly woken

## HTTP API

Sleep mode exposes HTTP endpoints:

```bash
# Put mesh to sleep
curl -X POST http://localhost:8080/sleep

# Wake the mesh
curl -X POST http://localhost:8080/wake

# Check status
curl http://localhost:8080/sleep/status
```

## Scheduled Sleep

Use cron to schedule sleep/wake cycles:

```bash
# Sleep at 6 PM
0 18 * * * curl -X POST http://localhost:8080/sleep

# Wake at 8 AM
0 8 * * * curl -X POST http://localhost:8080/wake
```

## Example Configurations

### Standard Sleep Mode

```yaml
sleep:
  enabled: true
  poll_interval: 5m
  poll_interval_jitter: 0.3
  poll_duration: 30s
```

### High-Frequency Polling

For environments where timely responsiveness matters:

```yaml
sleep:
  enabled: true
  poll_interval: 1m
  poll_interval_jitter: 0.2
  poll_duration: 15s
```

### Low-Profile Covert Mode

Minimize network activity:

```yaml
sleep:
  enabled: true
  poll_interval: 30m
  poll_interval_jitter: 0.5
  poll_duration: 20s
  auto_sleep_on_start: true
```

## Limitations

- **Command propagation**: Sleep/wake commands only reach connected agents
- **Isolated agents**: Agents not connected when sleep is triggered remain awake
- **Poll timing**: Sleeping agents may miss real-time events between polls
- **Trusted mesh**: Sleep/wake commands flood to all agents - use in trusted environments

## Troubleshooting

### Agent Not Entering Sleep

- Verify `sleep.enabled: true` in configuration
- Check that the HTTP API is accessible
- Review agent logs for sleep-related messages

### Wake Command Not Propagating

- Ensure at least one agent is awake or polling
- Check that agents have matching peer configurations
- Verify network connectivity between agents

### State Not Persisting

- Confirm `persist_state: true` in configuration
- Check that the data directory is writable
- Verify the agent has clean shutdown (state saved on stop)
