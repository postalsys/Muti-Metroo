---
title: Quick Start
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-escalator.png" alt="Mole on the move" style={{maxWidth: '180px'}} />
</div>

# Quick Start

This guide walks you through manually setting up a Muti Metroo agent with full control over the configuration.

## Step 1: Initialize Agent Identity

Each agent needs a unique identity. Create one:

```bash
muti-metroo init -d ./data
```

This generates a 128-bit Agent ID stored in `./data/agent_id`:

```
Agent ID: a1b2c3d4e5f6789012345678901234ab
```

:::tip
Save this Agent ID - you will need it when connecting other agents to this one.
:::

## Step 2: Generate TLS Certificates

All peer connections require TLS. Generate a Certificate Authority and agent certificates:

```bash
# Create Certificate Authority
muti-metroo cert ca -n "My Mesh CA" -o ./certs

# Generate agent certificate (signed by the CA)
muti-metroo cert agent -n "my-agent" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  --dns "agent1.example.com" \
  --ip "192.168.1.10" \
  -o ./certs

# Verify the certificate
muti-metroo cert info ./certs/my-agent.crt
```

Output files:
- `./certs/ca.crt` - CA certificate (share with peers)
- `./certs/ca.key` - CA private key (keep secure!)
- `./certs/my-agent.crt` - Agent certificate (named after common name)
- `./certs/my-agent.key` - Agent private key

:::warning
Keep `ca.key` secure. Anyone with access to it can create valid certificates for your mesh.
:::

## Step 3: Create Configuration File

Create `config.yaml` with your agent settings:

```yaml
# Agent identity
agent:
  id: "auto"                    # Uses ID from data directory
  display_name: "My First Agent"
  data_dir: "./data"
  log_level: "info"
  log_format: "text"

# Global TLS configuration
tls:
  cert: "./certs/my-agent.crt"
  key: "./certs/my-agent.key"

# Listen for peer connections
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    # Uses global TLS settings

# SOCKS5 proxy (ingress role)
socks5:
  enabled: true
  address: "127.0.0.1:1080"

# HTTP API (health checks, metrics, CLI)
http:
  enabled: true
  address: ":8080"
```

## Step 4: Run the Agent

Start your agent:

```bash
muti-metroo run -c ./config.yaml
```

You should see output like:

```
INFO  Starting Muti Metroo agent
INFO  Agent ID: a1b2c3d4e5f6789012345678901234ab
INFO  QUIC listener started on 0.0.0.0:4433
INFO  SOCKS5 server started on 127.0.0.1:1080
INFO  HTTP server started on :8080
INFO  Agent ready
```

## Step 5: Verify the Agent

In another terminal, test your agent:

```bash
# Check health
curl http://localhost:8080/health
# Output: OK

# Get detailed status
curl http://localhost:8080/healthz
# Output: {"status":"healthy","agent_id":"a1b2c3d4...","peers":0,"streams":0,"routes":0}

# View metrics
curl http://localhost:8080/metrics | head -20
```

## Step 6: Test SOCKS5 Proxy

With no exit routes configured, SOCKS5 connections will fail with "no route". But you can verify the proxy is working:

```bash
# This will fail with "no route to host" - that is expected!
curl -x socks5://localhost:1080 https://example.com

# Error: Connection refused (no exit node configured)
```

To actually proxy traffic, you need to either:
1. Enable exit on this agent (see [Exit Routing](../features/exit-routing))
2. Connect to another agent that has exit enabled

## Adding Exit Capability

To make this agent also serve as an exit node, add to your `config.yaml`:

```yaml
# Exit node configuration
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"          # Advertise default route
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

Restart the agent:

```bash
# Stop with Ctrl+C, then restart
muti-metroo run -c ./config.yaml
```

Now test the proxy:

```bash
# Should work now!
curl -x socks5://localhost:1080 https://example.com
```

## Configuration Summary

Here is the complete configuration for a single agent acting as both ingress and exit:

```yaml
agent:
  id: "auto"
  display_name: "All-in-One Agent"
  data_dir: "./data"
  log_level: "info"
  log_format: "text"

tls:
  cert: "./certs/my-agent.crt"
  key: "./certs/my-agent.key"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    # Uses global TLS settings

socks5:
  enabled: true
  address: "127.0.0.1:1080"

exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s

http:
  enabled: true
  address: ":8080"
```

## Next Steps

- [Your First Mesh](first-mesh) - Connect multiple agents
- [Configuration Reference](../configuration/overview) - All configuration options
- [SOCKS5 Proxy](../features/socks5-proxy) - Authentication and advanced usage
- [Exit Routing](../features/exit-routing) - Route configuration and DNS

## Troubleshooting

### Agent won't start

Check for common issues:

```bash
# Check if port is already in use
lsof -i :4433
lsof -i :1080
lsof -i :8080

# Check file permissions
ls -la ./data/
ls -la ./certs/

# Enable debug logging in config.yaml:
# agent:
#   log_level: "debug"
muti-metroo run -c ./config.yaml
```

### Certificate errors

```bash
# Verify certificate
muti-metroo cert info ./certs/agent.crt

# Check certificate expiration
openssl x509 -in ./certs/agent.crt -text -noout | grep -A2 "Validity"
```

### SOCKS5 connection refused

- Ensure SOCKS5 is enabled in config
- Check the bind address (use `127.0.0.1` for local-only, `0.0.0.0` for network access)
- Verify firewall rules

See [Troubleshooting](../troubleshooting/common-issues) for more help.
