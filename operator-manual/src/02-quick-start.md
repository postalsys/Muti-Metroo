# Quick Start

Get Muti Metroo running in under 5 minutes using the interactive setup wizard.

## Download

Download the binary for your platform:

```bash
# Linux (amd64)
curl -L -o muti-metroo \
  https://github.com/postalsys/Muti-Metroo/releases/latest/download/muti-metroo-linux-amd64

# Linux (arm64)
curl -L -o muti-metroo \
  https://github.com/postalsys/Muti-Metroo/releases/latest/download/muti-metroo-linux-arm64

# macOS (Apple Silicon)
curl -L -o muti-metroo \
  https://github.com/postalsys/Muti-Metroo/releases/latest/download/muti-metroo-darwin-arm64

# macOS (Intel)
curl -L -o muti-metroo \
  https://github.com/postalsys/Muti-Metroo/releases/latest/download/muti-metroo-darwin-amd64
```

Make it executable and install:

```bash
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Verify installation
muti-metroo --version
```

## Run the Setup Wizard

The easiest way to get started is using the interactive setup wizard:

```bash
muti-metroo setup
```

The wizard guides you through:

1. **Basic Setup** - Data directory and config file location
2. **Agent Identity** - Auto-generate or specify a custom ID
3. **Agent Role** - Ingress, Transit, Exit, or Combined
4. **Transport Configuration** - QUIC, HTTP/2, or WebSocket
5. **TLS Certificates** - Generate new or use existing
6. **Peer Connections** - Connect to other agents
7. **SOCKS5 Configuration** - If ingress role selected
8. **Exit Configuration** - If exit role selected
9. **Review and Save** - Confirm configuration

## Start the Agent

After the wizard completes:

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

## Verify Operation

In another terminal:

```bash
# Check health
curl http://localhost:8080/health
# Output: OK

# Get detailed status
curl http://localhost:8080/healthz
# Output: {"status":"healthy","agent_id":"...","peers":0,"streams":0,"routes":0}

# View web dashboard
open http://localhost:8080/ui/
```

## Test SOCKS5 Proxy

If you configured exit routing (standalone mode):

```bash
# Test the proxy
curl -x socks5://localhost:1080 https://example.com

# Test with authentication (if configured)
curl -x socks5://user:password@localhost:1080 https://example.com
```

## Minimal Configuration Example

For reference, here is a minimal standalone configuration (ingress + exit):

```yaml
agent:
  id: "auto"
  display_name: "Standalone Agent"
  data_dir: "./data"
  log_level: "info"

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
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 5s

http:
  enabled: true
  address: ":8080"
```

## Next Steps

- **Chapter 3**: Detailed installation options
- **Chapter 4**: Setup wizard walkthrough
- **Chapter 5**: TLS certificate management
- **Chapter 6**: Full configuration reference
