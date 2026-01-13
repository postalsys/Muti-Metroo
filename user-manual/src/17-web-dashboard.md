# Web Dashboard

Muti Metroo includes an embedded web dashboard with metro map visualization for monitoring mesh topology.

## Accessing the Dashboard

Open in a web browser:

```bash
open http://localhost:8080/ui/
```

Or navigate to `http://<agent-address>:8080/ui/`

## Features

### Metro Map Visualization

The dashboard displays the mesh topology as an interactive metro map:

- **Nodes**: Each agent is displayed as a station
- **Connections**: Peer connections shown as metro lines
- **Roles**: Color-coded rings indicate agent roles
  - Blue: Ingress (SOCKS5 enabled)
  - Gray: Transit (relay only)
  - Green: Exit (has routes)

### Agent Information

Hover over any agent node to see:

- Agent ID
- Display name
- Connected peers
- Exit routes (if applicable)
- SOCKS5 address (for ingress agents)
- System information (OS, hostname, if available)

### Real-time Updates

The dashboard automatically refreshes topology data to reflect:

- New peer connections
- Disconnected peers
- Route changes
- Agent status updates

## Configuration

Enable or disable the dashboard:

```yaml
http:
  enabled: true
  address: ":8080"
  dashboard: true            # Enable web dashboard
```

To disable:

```yaml
http:
  enabled: true
  address: ":8080"
  dashboard: false           # Disable web dashboard
```

## API Endpoints

The dashboard uses these JSON API endpoints:

### GET /api/topology

Returns topology data for the metro map:

```bash
curl http://localhost:8080/api/topology | jq
```

### GET /api/dashboard

Returns complete dashboard data:

```bash
curl http://localhost:8080/api/dashboard | jq
```

Response includes:
- Agent information
- Statistics (peers, streams, routes)
- Peer list with connection status
- Route table

### GET /api/nodes

Returns detailed information about all known nodes:

```bash
curl http://localhost:8080/api/nodes | jq
```

## Security Considerations

The dashboard exposes topology information that may be sensitive:

1. **Bind to localhost**: Use `127.0.0.1:8080` instead of `0.0.0.0:8080`
2. **Disable in production**: Set `dashboard: false` for remote agents
3. **Use management keys**: Encrypt node info with management key encryption

### Minimal Configuration

For remote agents, disable the dashboard:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  minimal: true              # Only health endpoints
```

For management stations, enable full dashboard:

```yaml
http:
  enabled: true
  address: "127.0.0.1:8080"
  dashboard: true
  remote_api: true

management:
  public_key: "..."
  private_key: "..."         # Operators can decrypt node info
```

## Troubleshooting

### Dashboard Not Loading

1. Verify HTTP API is enabled:
   ```yaml
   http:
     enabled: true
     dashboard: true
   ```

2. Check the address binding:
   ```bash
   curl http://localhost:8080/health
   ```

3. Verify firewall allows access to port 8080

### Nodes Not Appearing

1. Check peer connections:
   ```bash
   curl http://localhost:8080/healthz | jq '.peer_count'
   ```

2. Verify route propagation:
   ```bash
   curl -X POST http://localhost:8080/routes/advertise
   ```

3. Wait for node info advertisement (default: 2 minutes)

### Encrypted Node Info

If using management keys, remote agents without the private key will show limited information. Configure the private key on operator nodes:

```yaml
management:
  public_key: "..."
  private_key: "..."
```
