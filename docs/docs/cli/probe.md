---
title: probe
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-binoculars.png" alt="Mole probing" style={{maxWidth: '180px'}} />
</div>

# muti-metroo probe

Test connectivity to a Muti Metroo listener.

## Synopsis

```bash
muti-metroo probe [flags] <address>
```

## Description

The `probe` command tests if a Muti Metroo listener is reachable and responding. It performs two levels of verification:

1. **Transport-level connection** - Establishes a TCP/TLS connection using the specified transport (QUIC, HTTP/2, or WebSocket)
2. **Protocol handshake** - Performs a PEER_HELLO exchange to verify it's a real Muti Metroo listener

This command is useful for:

- Verifying connectivity before deploying an agent
- Diagnosing connection issues with existing listeners
- Testing firewall rules
- Validating TLS certificate configuration

**Note:** The probe operates standalone and does not require a running agent.

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--transport` | `-T` | `quic` | Transport type: `quic`, `h2`, `ws` |
| `--path` | | `/mesh` | HTTP path for h2/ws transports |
| `--timeout` | `-t` | `10s` | Connection timeout |
| `--ca` | | | CA certificate file for TLS verification |
| `--cert` | | | Client certificate file for mTLS |
| `--key` | | | Client key file for mTLS |
| `--insecure` | | `false` | Skip TLS certificate verification |
| `--json` | | `false` | Output results as JSON |
| `-h, --help` | | | Show help |

## Examples

### Test a QUIC Listener

QUIC is the default transport:

```bash
muti-metroo probe server.example.com:4433
```

Output on success:

```
Probing quic://server.example.com:4433...

[OK] Connection successful!
  Transport:    quic
  Address:      server.example.com:4433
  Remote ID:    abc123def456789012345678
  Display Name: Agent-B
  RTT:          23ms
```

### Test an HTTP/2 Listener

```bash
muti-metroo probe --transport h2 server.example.com:443 --path /mesh
```

### Test a WebSocket Listener

```bash
muti-metroo probe --transport ws server.example.com:443 --path /mesh
```

### Test with Self-Signed Certificate

When testing against a listener with a self-signed certificate:

```bash
muti-metroo probe --insecure server.example.com:4433
```

### Test with Specific CA Certificate

When using a private CA:

```bash
muti-metroo probe --ca ./certs/ca.crt server.example.com:4433
```

### Test with mTLS (Mutual TLS)

When the listener requires client authentication:

```bash
muti-metroo probe --ca ./certs/ca.crt --cert ./certs/client.crt --key ./certs/client.key server.example.com:4433
```

### JSON Output for Scripting

```bash
muti-metroo probe --json server.example.com:4433
```

Output:

```json
{
  "success": true,
  "transport": "quic",
  "address": "server.example.com:4433",
  "remote_id": "abc123def456789012345678",
  "remote_display_name": "Agent-B",
  "rtt_ms": 23
}
```

### Failed Connection Example

```bash
muti-metroo probe server.example.com:4433
```

Output on failure:

```
Probing quic://server.example.com:4433...

[FAILED] Connection failed
  Transport:  quic
  Address:    server.example.com:4433
  Error:      Connection timed out - firewall may be blocking
```

## Error Messages

The probe provides helpful error messages for common failure scenarios:

| Error Message | Likely Cause |
|---------------|--------------|
| Could not resolve hostname | DNS lookup failed |
| Connection refused - listener not running or port blocked | No process listening on the port |
| Connection timed out - firewall may be blocking | Firewall dropping packets |
| TLS error - certificate signed by unknown authority | Need `--ca` or `--insecure` |
| TLS error - certificate has expired | Listener certificate expired |
| Connected but handshake failed - not a Muti Metroo listener? | Port is open but not a Muti Metroo listener |
| Connected but received invalid response | Protocol mismatch |

## Use Cases

### Pre-deployment Verification

Before deploying an agent to a target machine, verify the listener is reachable:

```bash
# On the target machine (or a machine with similar network access)
muti-metroo probe --insecure relay.example.com:4433
```

### Firewall Troubleshooting

Test different transports to identify what's blocked:

```bash
# Test QUIC (UDP)
muti-metroo probe --transport quic server.example.com:4433

# Test HTTP/2 (TCP, HTTPS)
muti-metroo probe --transport h2 server.example.com:443

# Test WebSocket (TCP, HTTPS)
muti-metroo probe --transport ws server.example.com:443
```

### TLS Configuration Validation

Verify TLS is configured correctly:

```bash
# Should work with proper CA
muti-metroo probe --ca ./certs/ca.crt server.example.com:4433

# Should fail without CA (unless using --insecure)
muti-metroo probe server.example.com:4433
```

## Wizard Integration

The setup wizard automatically tests peer connectivity when configuring peer connections. After entering peer details, the wizard will probe the listener and show results:

```
[INFO] Testing connectivity to peer...
[OK] Connected successfully!
[INFO] Remote agent: Agent-B (abc123def456)
[INFO] Round-trip time: 23ms
```

If the connection fails, you can choose to:

1. Continue anyway (set up the listener later)
2. Retry the connection test
3. Re-enter peer configuration
4. Skip this peer

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Probe successful |
| 1 | Probe failed or error |

## Related

- [Setup Wizard](/cli/setup) - Interactive agent configuration
- [Transports](/concepts/transports) - Transport protocol details
- [TLS Certificates](/configuration/tls-certificates) - TLS certificate setup
- [Troubleshooting](/troubleshooting/common-issues) - Common issues and solutions
