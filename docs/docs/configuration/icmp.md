---
title: ICMP Configuration
sidebar_position: 8
---

# ICMP Configuration

Configure ICMP echo (ping) support on exit agents. When enabled, agents can forward ICMP echo requests to allowed destinations.

## Overview

ICMP support allows you to ping remote hosts through the mesh network. The feature:

- Uses **unprivileged ICMP sockets** (no root required on most systems)
- Supports **E2E encryption** for all traffic through the mesh
- Provides **CIDR-based access control** to limit ping targets
- Works with the `muti-metroo ping` CLI command

## Configuration Options

```yaml
icmp:
  enabled: false           # Enable ICMP echo forwarding
  allowed_cidrs:           # CIDRs that can be pinged
    - "0.0.0.0/0"         # Allow all IPv4
  max_sessions: 100        # Concurrent session limit (0 = unlimited)
  idle_timeout: 60s        # Session cleanup timeout
  echo_timeout: 10s        # Per-echo request timeout
```

### enabled

Controls whether ICMP echo forwarding is available on this exit agent.

| Type | Default |
|------|---------|
| bool | `false` |

When disabled, ping requests to this agent will fail with "icmp not enabled".

### allowed_cidrs

List of CIDR ranges that can be pinged through this exit agent.

| Type | Default |
|------|---------|
| []string | `[]` (none) |

Examples:

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    # Allow all IPv4 addresses
    - "0.0.0.0/0"
```

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    # Only internal networks
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    # Specific subnet only
    - "192.168.1.0/24"
```

If a ping target is not in any allowed CIDR, the request fails with "destination not allowed".

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

### Allow All (Testing)

For testing environments where you want to allow pinging any destination:

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    - "0.0.0.0/0"
```

### Internal Networks Only

Restrict to RFC 1918 private address ranges:

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
  max_sessions: 50
  idle_timeout: 30s
```

### Specific Targets

Allow only specific network segments:

```yaml
icmp:
  enabled: true
  allowed_cidrs:
    - "192.168.100.0/24"   # Server subnet
    - "192.168.200.0/24"   # Management subnet
  max_sessions: 20
```

### Disabled (Default)

ICMP is disabled by default for security:

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

1. **Disabled by default**: ICMP is off unless explicitly enabled
2. **CIDR filtering**: Use `allowed_cidrs` to restrict ping targets
3. **No domain resolution**: Only IP addresses are accepted (no DNS)
4. **E2E encryption**: All ICMP data is encrypted through the mesh
5. **Session limits**: Use `max_sessions` to prevent resource exhaustion

## Usage

Once configured, use the CLI to ping through the exit agent:

```bash
# Basic ping
muti-metroo ping <exit-agent-id> 8.8.8.8

# Continuous ping
muti-metroo ping -c 0 <exit-agent-id> 192.168.1.1
```

See [ping CLI command](/cli/ping) for full usage details.

## Related

- [ping CLI Command](/cli/ping) - Send ICMP requests through agents
- [Exit Configuration](/configuration/exit) - Configure exit routing
- [UDP Configuration](/configuration/udp) - UDP relay configuration
