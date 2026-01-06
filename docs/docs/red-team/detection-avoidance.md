---
title: Detection Avoidance
sidebar_label: Detection Avoidance
sidebar_position: 8
---

# Detection Avoidance

Techniques for minimizing detection signatures on the network and host.

## Network Indicators

| Indicator | Default | Mitigation |
|-----------|---------|------------|
| Custom ALPN | `muti-metroo/1` | Set empty string |
| HTTP header | `X-Muti-Metroo-Protocol` | Set empty string |
| WS subprotocol | `muti-metroo/1` | Set empty string |
| Certificate CN | `muti-metroo` | Use realistic names |
| Beaconing interval | 30s keepalive | Configure `keepalive_jitter` (default 0.2 = 20%) |
| Connection burst | Immediate | Stagger peer connections |

### Keepalive Jitter

Fixed-interval keepalives create detectable beacon patterns. Muti Metroo applies timing jitter to randomize keepalive intervals:

```yaml
connections:
  idle_threshold: 30s      # Base keepalive interval
  keepalive_jitter: 0.2    # 20% jitter (24-36s range)
```

Higher jitter values provide better evasion but may affect connection stability:

| Jitter | Range (30s base) | Use Case |
|--------|------------------|----------|
| 0.0 | Fixed 30s | Testing only (detectable) |
| 0.2 | 24-36s | Default (balanced) |
| 0.3 | 21-39s | Enhanced evasion |
| 0.5 | 15-45s | Maximum evasion (may affect stability) |

## Host Indicators

| Indicator | Mitigation |
|-----------|------------|
| Binary name | Rename to match environment |
| Service name | Customize service installation |
| Config path | Use realistic system paths |
| Log files | Set `log_level: error`, use syslog |
| Data directory | Blend with system directories |

## Certificate Considerations

Generate certificates with realistic attributes:

```bash
# Generate CA with corporate-like name
muti-metroo cert ca --cn "Internal Services Root CA" -o ./certs

# Generate agent cert matching environment
muti-metroo cert agent --cn "api-gateway-prod-01" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs
```

**Certificate tips:**
- Match organizational naming conventions
- Use appropriate validity periods (1 year typical)
- Consider using legitimate certificates if available
- Self-signed certs may trigger TLS inspection alerts
