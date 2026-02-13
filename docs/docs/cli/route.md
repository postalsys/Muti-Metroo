# Route Commands

Commands for managing dynamic CIDR exit routes.

## route add

Add a dynamic CIDR exit route.

```bash
muti-metroo route add <cidr> [flags]
```

### Description

Adds a new exit route for the specified CIDR range. The route is added to the target agent's exit route table and advertised to the mesh.

Dynamic routes are ephemeral and lost on restart. For persistent routes, use the `exit.routes` configuration.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent API address |
| `--target` | `-t` | | Target agent ID (omit for local agent) |
| `--metric` | `-m` | `0` | Route metric |

### Examples

```bash
# Add route on local agent
muti-metroo route add 10.0.0.0/24

# Add route with metric
muti-metroo route add 10.0.0.0/24 -m 5

# Add route on remote agent
muti-metroo route add 10.0.0.0/24 -t abc123def456

# Add route using short agent ID prefix
muti-metroo route add 10.0.0.0/24 -t abc123

# Via a specific API server
muti-metroo route add 10.0.0.0/24 -a 192.168.1.10:8080 -t def456
```

### Output

```
Route added: 10.0.0.0/24 (metric 0)
```

### Use Cases

- **Transit to Exit Promotion**: Convert a transit-only agent to an exit agent on the fly
- **Dynamic Route Injection**: Add routes based on runtime conditions or automation
- **Testing**: Temporarily expose routes without modifying configuration files
- **Emergency Routing**: Add fallback routes during network issues

---

## route remove

Remove a dynamic CIDR exit route.

```bash
muti-metroo route remove <cidr> [flags]
```

### Description

Removes a previously added dynamic exit route. The route is removed from the target agent's exit route table and the change is advertised to the mesh.

Only dynamic routes can be removed via this command. Routes defined in the `exit.routes` configuration are persistent and cannot be removed without restarting the agent.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent API address |
| `--target` | `-t` | | Target agent ID (omit for local agent) |

### Examples

```bash
# Remove route from local agent
muti-metroo route remove 10.0.0.0/24

# Remove route from remote agent
muti-metroo route remove 10.0.0.0/24 -t abc123def456

# Remove route using short agent ID prefix
muti-metroo route remove 10.0.0.0/24 -t abc123

# Via a specific API server
muti-metroo route remove 10.0.0.0/24 -a 192.168.1.10:8080 -t def456
```

### Output

```
Route removed: 10.0.0.0/24
```

---

## route list

List dynamic CIDR exit routes.

```bash
muti-metroo route list [flags]
```

### Description

Displays all dynamic exit routes currently active on the target agent. Routes configured in `exit.routes` are not shown by this command.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent API address |
| `--target` | `-t` | | Target agent ID (omit for local agent) |
| `--json` | | `false` | Output in JSON format |

### Examples

```bash
# List routes on local agent
muti-metroo route list

# List routes on remote agent
muti-metroo route list -t abc123def456

# List routes with JSON output
muti-metroo route list --json

# Via a specific API server
muti-metroo route list -a 192.168.1.10:8080 -t def456
```

### Output

Standard output:

```
Dynamic Exit Routes
===================
10.0.0.0/24 (metric 0)
192.168.1.0/24 (metric 5)
```

JSON output:

```json
{
  "routes": [
    {
      "cidr": "10.0.0.0/24",
      "metric": 0
    },
    {
      "cidr": "192.168.1.0/24",
      "metric": 5
    }
  ]
}
```

---

## Authorization

### Management Key Restriction

Dynamic route modifications require management key authorization when the mesh is configured with topology encryption.

If an agent has a `management_public_key` configured, route add/remove commands are only accepted from clients presenting the corresponding private key. This restricts who can modify the mesh topology.

```yaml
# In the agent configuration
management:
  management_public_key: "base64-encoded-public-key"
```

Without authorization, add/remove commands return HTTP 403 Forbidden.

### Generating Management Keys

```bash
# Generate keypair
muti-metroo management-key generate > private.key

# Derive public key for distribution
muti-metroo management-key public < private.key
```

---

## Important Notes

### Ephemeral Routes

Dynamic routes added via `route add` are ephemeral and lost when the agent restarts. For persistent routes, use the `exit.routes` configuration:

```yaml
exit:
  routes:
    - 10.0.0.0/24
    - 192.168.1.0/24
```

### Route Propagation

After adding or removing a route, the change is advertised to the mesh during the next route advertisement interval (default 2 minutes). To trigger immediate propagation:

```bash
curl -X POST http://localhost:8080/routes/advertise
```

### Short Agent ID Prefixes

The `--target` flag accepts short agent ID prefixes. If multiple agents match the prefix, the command fails. Provide a longer prefix to disambiguate.

```bash
# Agent IDs: abc123def456, abc789ghi012

# Ambiguous
muti-metroo route add 10.0.0.0/24 -t abc
# Error: multiple agents match prefix

# Specific
muti-metroo route add 10.0.0.0/24 -t abc123
# Success
```

---

## Workflow Examples

### Promote Transit Agent to Exit

Convert a transit-only agent to an exit agent at runtime:

```bash
# Add default route
muti-metroo route add 0.0.0.0/0 -t abc123def456

# Verify route
muti-metroo route list -t abc123def456
```

### Automation Script

Add routes based on cloud provider metadata:

```bash
#!/bin/bash
# Detect VPC CIDR and advertise it
VPC_CIDR=$(curl -s http://169.254.169.254/latest/meta-data/network/interfaces/macs/$(curl -s http://169.254.169.254/latest/meta-data/mac)/vpc-ipv4-cidr-block)
muti-metroo route add "$VPC_CIDR"
```

### Testing Exit Routing

Temporarily add a route for testing:

```bash
# Add test route
muti-metroo route add 10.99.0.0/16 -t testnode

# Test connectivity
curl --socks5 localhost:1080 http://10.99.1.1/test

# Remove route
muti-metroo route remove 10.99.0.0/16 -t testnode
```

### Emergency Fallback

Add a backup route during network issues:

```bash
# Primary exit agent down, add fallback
muti-metroo route add 0.0.0.0/0 -t backup-exit -m 100

# When primary recovers, remove fallback
muti-metroo route remove 0.0.0.0/0 -t backup-exit
```
