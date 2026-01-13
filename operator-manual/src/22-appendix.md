# Appendix

## CLI Quick Reference

### Getting Started

| Command | Description |
|---------|-------------|
| `muti-metroo setup` | Interactive setup wizard |
| `muti-metroo init -d ./data` | Initialize agent identity |
| `muti-metroo run -c config.yaml` | Run agent with config |

### Agent Status

| Command | Description |
|---------|-------------|
| `muti-metroo status` | Show agent status |
| `muti-metroo peers` | List connected peers |
| `muti-metroo routes` | List route table |
| `muti-metroo probe <address>` | Test connectivity to listener |

### Remote Operations

| Command | Description |
|---------|-------------|
| `muti-metroo shell <id> <cmd>` | Execute remote command |
| `muti-metroo shell --tty <id> bash` | Interactive shell |
| `muti-metroo upload <id> <local> <remote>` | Upload file |
| `muti-metroo download <id> <remote> <local>` | Download file |

### Administration

| Command | Description |
|---------|-------------|
| `muti-metroo service install -c config.yaml` | Install as service |
| `muti-metroo service uninstall` | Uninstall service |
| `muti-metroo service status` | Check service status |
| `muti-metroo cert ca --cn "CA"` | Generate CA certificate |
| `muti-metroo cert agent --cn "name"` | Generate agent certificate |
| `muti-metroo cert client --cn "name"` | Generate client certificate |
| `muti-metroo cert info <cert>` | Display certificate info |
| `muti-metroo hash` | Generate bcrypt password hash |
| `muti-metroo management-key generate` | Generate management keypair |

## Configuration Cheatsheet

### Minimal Standalone

```yaml
agent:
  id: "auto"
  data_dir: "./data"

tls:
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

socks5:
  enabled: true
  address: "127.0.0.1:1080"

exit:
  enabled: true
  routes:
    - "0.0.0.0/0"

http:
  enabled: true
  address: ":8080"
```

### Ingress Only

```yaml
agent:
  display_name: "Ingress"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

peers:
  - id: "exit-agent-id..."
    transport: quic
    address: "exit.example.com:4433"

socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "operator"
        password_hash: "$2a$12$..."
```

### Exit Only

```yaml
agent:
  display_name: "Exit"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"

exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
```

### Minimal Configuration

```yaml
protocol:
  alpn: "h2"
  http_header: ""
  ws_subprotocol: ""

http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true

management:
  public_key: "${MGMT_PUBKEY}"
```

## Default Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 4433 | UDP | QUIC transport |
| 8443 | TCP | HTTP/2 transport |
| 443 | TCP | WebSocket transport |
| 1080 | TCP | SOCKS5 proxy |
| 8080 | TCP | HTTP API |

## HTTP API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Basic health check |
| `/healthz` | GET | Detailed health JSON |
| `/ready` | GET | Readiness probe |
| `/ui/` | GET | Web dashboard |
| `/api/topology` | GET | Topology data |
| `/api/dashboard` | GET | Dashboard data |
| `/api/nodes` | GET | Node list |
| `/agents` | GET | List all agents |
| `/agents/{id}` | GET | Agent status |
| `/agents/{id}/routes` | GET | Agent routes |
| `/agents/{id}/peers` | GET | Agent peers |
| `/agents/{id}/shell` | GET | WebSocket shell |
| `/agents/{id}/file/upload` | POST | Upload file |
| `/agents/{id}/file/download` | POST | Download file |
| `/routes/advertise` | POST | Trigger route advertisement |

## Environment Variables

Syntax:
- `${VAR}` - Required variable
- `${VAR:-default}` - With default value

Common usage:

```yaml
agent:
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"

socks5:
  auth:
    users:
      - username: "${SOCKS_USER}"
        password_hash: "${SOCKS_PASS_HASH}"

management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```

## Transport Comparison

| Transport | Protocol | Performance | Firewall |
|-----------|----------|-------------|----------|
| QUIC | UDP | Best | Medium |
| HTTP/2 | TCP | Good | Good |
| WebSocket | TCP | Fair | Excellent |

## Role Summary

| Role | Config Section | Purpose |
|------|----------------|---------|
| Ingress | `socks5:` | Accept client connections |
| Transit | (implicit) | Relay between peers |
| Exit | `exit:` | Open external connections |

## Certificate Commands

```bash
# Create CA
muti-metroo cert ca --cn "Mesh CA" -o ./certs

# Create agent cert
muti-metroo cert agent --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  --dns agent1.example.com \
  --ip 192.168.1.10 \
  -o ./certs

# View cert info
muti-metroo cert info ./certs/agent-1.crt
```

## Hash Generation

```bash
# Interactive
muti-metroo hash

# With custom cost
muti-metroo hash --cost 12

# From argument (not recommended)
muti-metroo hash "password"
```

## Management Key Generation

```bash
# Generate keypair
muti-metroo management-key generate

# Derive public key from private
muti-metroo management-key public --private <private-key>
```

## Useful Commands

### Health Check

```bash
curl http://localhost:8080/health
curl http://localhost:8080/healthz | jq
```

### Trigger Route Advertisement

```bash
curl -X POST http://localhost:8080/routes/advertise
```

### Test SOCKS5

```bash
curl -x socks5://localhost:1080 https://example.com
curl -x socks5://user:pass@localhost:1080 https://example.com
```

### SSH Through Proxy

```bash
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@host
```

### Probe Connectivity

```bash
muti-metroo probe -T quic target.example.com:4433
muti-metroo probe -T h2 target.example.com:8443
muti-metroo probe -T ws target.example.com:443
```
