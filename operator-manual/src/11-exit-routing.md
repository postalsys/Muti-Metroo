# Exit Routing

Exit nodes advertise routes and open TCP connections to external destinations. Two types of routes are supported:

- **CIDR routes**: Match destinations by IP address (e.g., `10.0.0.0/8`)
- **Domain routes**: Match destinations by domain name (e.g., `*.example.com`)

**Note:** Domain routes only work with **SOCKS5 clients** that send the destination hostname. The Mutiauk TUN interface operates at Layer 3 (IP) and only sees IP addresses after DNS resolution, so it can only use **CIDR routes**. Mutiauk's autoroutes feature fetches only CIDR routes from Muti Metroo.

## Configuration

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "0.0.0.0/0"         # Default route
  domain_routes:
    - "api.internal.corp"  # Exact domain match
    - "*.example.com"      # Wildcard match
  dns:
    servers:
      - "8.8.8.8:53"
      - "1.1.1.1:53"
    timeout: 5s
```

## Route Types

### CIDR Routes

CIDR routes match destinations by IP address. DNS resolution happens at the **ingress** agent.

```yaml
exit:
  routes:
    - "10.0.0.0/8"        # Private network
    - "192.168.0.0/16"    # Another private range
    - "0.0.0.0/0"         # Default route (all traffic)
```

**Flow:**

1. Client connects via SOCKS5 with domain (e.g., `example.com`)
2. Ingress agent resolves domain using system DNS
3. Ingress performs route lookup using the resolved IP
4. Stream opens to exit node with the **IP address**
5. Exit opens TCP connection to the destination IP

### Domain Routes

Domain routes match destinations by domain name pattern. DNS resolution happens at the **exit** agent.

```yaml
exit:
  domain_routes:
    - "api.internal.corp"   # Exact match
    - "*.example.com"       # Wildcard match
```

**Flow:**

1. Client connects via SOCKS5 with domain (e.g., `api.internal.corp`)
2. Ingress checks domain routes first
3. If matched, stream opens to exit node with the **domain name**
4. Exit agent resolves domain using its configured DNS servers
5. Exit opens TCP connection to the resolved IP

**When to use domain routes:**
- **Split-horizon DNS**: Internal domains resolve differently inside vs. outside the network
- **Private services**: Route `*.internal.corp` to an exit with access to internal DNS
- **Geo-specific resolution**: Different DNS results based on exit location

## Route Selection

### Domain Route Priority

Domain routes are checked **first** for domain-based requests:

1. **Exact match**: `api.example.com` matches only `api.example.com`
2. **Wildcard match**: `*.example.com` matches `foo.example.com`, `bar.example.com`
3. If no domain route matches, fall back to CIDR routing

Wildcard matching is **single-level only**:
- `*.example.com` matches `foo.example.com`
- `*.example.com` does NOT match `a.b.example.com` or `example.com`

### CIDR Route Selection

Uses longest-prefix match:

1. Filter routes where CIDR contains destination IP
2. Select route with longest prefix (most specific)
3. If tied, select lowest metric (hop count)

Example with multiple exits:
- Exit A: `1.2.3.4/32` (metric 3) - Wins for `1.2.3.4`
- Exit B: `1.2.3.0/24` (metric 2) - Wins for `1.2.3.5`
- Exit C: `0.0.0.0/0` (metric 1) - Wins for everything else

## Route Advertisement

Routes are propagated through the mesh automatically:

- **Periodic**: Every `routing.advertise_interval` (default 2 minutes)
- **On-demand**: Via HTTP API

### Trigger Immediate Advertisement

```bash
curl -X POST http://localhost:8080/routes/advertise
```

This is useful after configuration changes to speed up route propagation.

## DNS Configuration

By default, the exit node uses the system resolver, which supports local domains (e.g., `printer.local`). You can optionally configure explicit DNS servers:

```yaml
exit:
  dns:
    servers:
      - "8.8.8.8:53"      # Primary
      - "1.1.1.1:53"      # Fallback
    timeout: 5s
```

For internal networks with custom DNS:

```yaml
exit:
  dns:
    servers:
      - "10.0.0.1:53"     # Internal DNS server
    timeout: 5s
```

## Access Control

Only destinations matching advertised routes are allowed:

```yaml
exit:
  routes:
    - "10.0.0.0/8"        # Only allow 10.x.x.x
```

Connections to other IPs will be rejected with "no route to host".

## Example Configurations

### Internet Gateway

Route all traffic to the internet:

```yaml
exit:
  enabled: true
  routes:
    - "0.0.0.0/0"
  dns:
    servers:
      - "8.8.8.8:53"
```

### Internal Network Access

Route only to internal networks:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
  dns:
    servers:
      - "10.0.0.1:53"
```

### Split-Horizon DNS

Route internal domains with internal DNS resolution:

```yaml
exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
  domain_routes:
    - "*.internal.corp"
    - "*.corp.local"
  dns:
    servers:
      - "10.0.0.1:53"
```

## Verifying Routes

Check advertised routes on any agent:

```bash
# View route table
curl http://localhost:8080/healthz | jq '.routes'

# Via CLI
muti-metroo routes -a localhost:8080
```
