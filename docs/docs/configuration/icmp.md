---
title: ICMP Configuration
sidebar_position: 8
---

# ICMP Configuration

Configure ICMP echo (ping) support on exit agents. When enabled, agents can forward ICMP echo requests to destinations.

## Overview

ICMP support allows you to ping remote hosts through the mesh network. The feature:

- Uses **unprivileged ICMP sockets** (no root required on most systems)
- Supports **E2E encryption** for all traffic through the mesh
- Works with the `muti-metroo ping` CLI command

## Configuration Options

```yaml
icmp:
  enabled: true            # Enable ICMP echo forwarding (default: true)
  max_sessions: 100        # Concurrent session limit (0 = unlimited)
  idle_timeout: 60s        # Session cleanup timeout
  echo_timeout: 10s        # Per-echo request timeout
```

### enabled

Controls whether ICMP echo forwarding is available on this exit agent.

| Type | Default |
|------|---------|
| bool | `true` |

When disabled, ping requests to this agent will fail with "icmp not enabled".

### max_sessions

Maximum number of concurrent ICMP sessions.

| Type | Default |
|------|---------|
| int | `100` |

Set to `0` for unlimited sessions. Each unique (destination IP, ICMP identifier) pair creates a session.

### idle_timeout

How long inactive sessions are kept before cleanup.

| Type | Default |
|------|---------|
| duration | `60s` |

Sessions are cleaned up after this duration of inactivity. Active sessions are kept alive as long as echo requests are being sent.

### echo_timeout

Timeout for individual ICMP echo requests.

| Type | Default |
|------|---------|
| duration | `10s` |

This is the server-side timeout. The CLI also has its own timeout (`-t` flag) which may be shorter.

## Example Configurations

### Default (Enabled)

ICMP is enabled by default with sensible limits:

```yaml
icmp:
  enabled: true
  max_sessions: 100
```

### High-Volume Testing

For testing environments with many concurrent pings:

```yaml
icmp:
  enabled: true
  max_sessions: 500
  idle_timeout: 30s
```

### Disabled

To disable ICMP echo forwarding:

```yaml
icmp:
  enabled: false
```

## Linux System Requirements

On Linux, unprivileged ICMP sockets require the `ping_group_range` sysctl to be configured:

```bash
# Check current setting
sysctl net.ipv4.ping_group_range

# Enable for all groups (run as root)
sudo sysctl -w net.ipv4.ping_group_range="0 65535"

# Make persistent
echo "net.ipv4.ping_group_range=0 65535" | sudo tee -a /etc/sysctl.conf
```

Without this setting, ICMP will fail with permission errors.

## Security Considerations

1. **E2E encryption**: All ICMP data is encrypted through the mesh
2. **Session limits**: Use `max_sessions` to prevent resource exhaustion
3. **No domain resolution**: Only IP addresses are accepted (no DNS)

## Usage

Once configured, use the CLI to ping through the exit agent:

```bash
# Basic ping
muti-metroo ping 8.8.8.8

# Continuous ping
muti-metroo ping -c 0 192.168.1.1
```

See [ping CLI command](/cli/ping) for full usage details.

## Related

- [ping CLI Command](/cli/ping) - Send ICMP requests through agents
- [Exit Configuration](/configuration/exit) - Configure exit routing
- [UDP Configuration](/configuration/udp) - UDP relay configuration
