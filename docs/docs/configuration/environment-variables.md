---
title: Environment Variables
sidebar_position: 8
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole reading environment" style={{maxWidth: '180px'}} />
</div>

# Environment Variable Substitution

Muti Metroo configuration files support environment variable substitution, allowing you to externalize secrets and deployment-specific values.

## Syntax

### Basic Substitution

```yaml
agent:
  log_level: "${LOG_LEVEL}"
```

If `LOG_LEVEL` is not set, the agent will fail to start.

### With Default Value

```yaml
agent:
  log_level: "${LOG_LEVEL:-info}"
```

Uses `info` if `LOG_LEVEL` is not set.

### Examples

```yaml
agent:
  id: "${AGENT_ID:-auto}"
  display_name: "${HOSTNAME:-unknown}"
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"

socks5:
  address: "${SOCKS_ADDR:-127.0.0.1:1080}"
  auth:
    users:
      - username: "${SOCKS_USER:-user}"
        password_hash: "${SOCKS_PASS_HASH}"

peers:
  - id: "${PEER_ID}"
    transport: "${PEER_TRANSPORT:-quic}"
    address: "${PEER_ADDR}"
```

## Common Use Cases

### Secrets

Never hardcode secrets in configuration:

```yaml
socks5:
  auth:
    users:
      - username: "admin"
        password_hash: "${SOCKS5_PASSWORD_HASH}"

rpc:
  password_hash: "${RPC_PASSWORD_HASH}"

file_transfer:
  password_hash: "${FILE_TRANSFER_PASSWORD_HASH}"
```

### TLS Certificates

Inline certificates from environment:

```yaml
listeners:
  - transport: quic
    tls:
      cert_pem: "${TLS_CERT}"
      key_pem: "${TLS_KEY}"
      client_ca_pem: "${TLS_CA}"
```

### Deployment Configuration

Different values per environment:

```yaml
agent:
  display_name: "${ENVIRONMENT:-dev}-${HOSTNAME}"

http:
  address: "${HTTP_ADDR:-:8080}"

listeners:
  - transport: quic
    address: "${LISTEN_ADDR:-0.0.0.0:4433}"
```

### Dynamic Peer Discovery

```yaml
peers:
  - id: "${PEER_1_ID}"
    address: "${PEER_1_ADDR}"
    transport: quic
    tls:
      ca: "${PEER_1_CA:-/etc/muti-metroo/ca.crt}"

  - id: "${PEER_2_ID}"
    address: "${PEER_2_ADDR}"
    transport: quic
    tls:
      ca: "${PEER_2_CA:-/etc/muti-metroo/ca.crt}"
```

## Running with Environment Variables

### Command Line

```bash
# Single variable
LOG_LEVEL=debug ./build/muti-metroo run -c config.yaml

# Multiple variables
SOCKS_ADDR=0.0.0.0:1080 \
LOG_LEVEL=info \
PEER_ID=abc123 \
./build/muti-metroo run -c config.yaml
```

### Environment File

Create `.env` file:

```bash
# .env
LOG_LEVEL=info
SOCKS_ADDR=0.0.0.0:1080
SOCKS5_PASSWORD_HASH=$2a$10$...
PEER_ID=abc123def456789012345678901234ab
PEER_ADDR=192.168.1.10:4433
```

Load and run:

```bash
# Using env command
env $(cat .env | xargs) ./build/muti-metroo run -c config.yaml

# Using set -a
set -a && source .env && set +a
./build/muti-metroo run -c config.yaml
```

### Docker

```bash
docker run -d \
  -e LOG_LEVEL=info \
  -e SOCKS_ADDR=0.0.0.0:1080 \
  -e PEER_ID=abc123... \
  -v ./config.yaml:/app/config.yaml \
  muti-metroo
```

Or with env file:

```bash
docker run -d \
  --env-file .env \
  -v ./config.yaml:/app/config.yaml \
  muti-metroo
```

### Docker Compose

```yaml
services:
  agent:
    image: muti-metroo
    environment:
      - LOG_LEVEL=info
      - SOCKS_ADDR=0.0.0.0:1080
    env_file:
      - .env
    volumes:
      - ./config.yaml:/app/config.yaml
```

### Kubernetes

ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muti-metroo-config
data:
  LOG_LEVEL: "info"
  SOCKS_ADDR: "0.0.0.0:1080"
```

Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: muti-metroo-secrets
type: Opaque
stringData:
  SOCKS5_PASSWORD_HASH: "$2a$10$..."
  TLS_KEY: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
```

Deployment:

```yaml
spec:
  containers:
    - name: muti-metroo
      envFrom:
        - configMapRef:
            name: muti-metroo-config
        - secretRef:
            name: muti-metroo-secrets
```

### systemd

```ini
# /etc/systemd/system/muti-metroo.service
[Service]
Environment="LOG_LEVEL=info"
Environment="SOCKS_ADDR=0.0.0.0:1080"
EnvironmentFile=/etc/muti-metroo/environment
ExecStart=/usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml
```

## Best Practices

### 1. Use Defaults for Non-Secrets

```yaml
# Good - has sensible default
log_level: "${LOG_LEVEL:-info}"

# Bad - will fail if not set
log_level: "${LOG_LEVEL}"
```

### 2. Never Default Secrets

```yaml
# Good - forces explicit configuration
password_hash: "${PASSWORD_HASH}"

# Bad - insecure default
password_hash: "${PASSWORD_HASH:-$2a$10$defaulthash...}"
```

### 3. Document Required Variables

```yaml
# Required environment variables:
# - PEER_ID: Agent ID of peer
# - PEER_ADDR: Address of peer
# - SOCKS5_PASSWORD_HASH: bcrypt hash for SOCKS5 auth
#
# Optional:
# - LOG_LEVEL: Logging level (default: info)
# - SOCKS_ADDR: SOCKS5 bind address (default: 127.0.0.1:1080)
```

### 4. Use Descriptive Names

```yaml
# Good
MUTI_METROO_LOG_LEVEL
MUTI_METROO_SOCKS5_PASSWORD_HASH

# OK for simple deployments
LOG_LEVEL
SOCKS5_PASSWORD_HASH
```

### 5. Validate Before Deployment

```bash
# Check all required vars are set
required_vars="PEER_ID PEER_ADDR SOCKS5_PASSWORD_HASH"
for var in $required_vars; do
  if [ -z "${!var}" ]; then
    echo "Error: $var is not set"
    exit 1
  fi
done
```

## Troubleshooting

### Missing Environment Variable

```
Error: environment variable PEER_ID not set
```

Set the variable or add a default value.

### Multiline Values

For multiline values (like PEM certificates):

```bash
# In shell
export TLS_CERT="$(cat ./certs/agent.crt)"

# In .env file (escape newlines)
TLS_CERT="-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----"
```

### Special Characters

Escape special characters in values:

```bash
# Password with special chars
SOCKS5_PASSWORD_HASH='$2a$10$abc123...'  # Use single quotes
```

## Related

- [Configuration Overview](overview) - Full configuration reference
- [Deployment: Docker](../deployment/docker) - Docker deployment
- [Deployment: Kubernetes](../deployment/kubernetes) - Kubernetes deployment
