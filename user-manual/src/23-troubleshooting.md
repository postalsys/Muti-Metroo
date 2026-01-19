# Troubleshooting

Quick solutions for frequently encountered problems.

## Agent Won't Start

### Port Already in Use

```
Error: listen tcp 0.0.0.0:4433: bind: address already in use
```

**Solution:**

```bash
# Find what's using the port
lsof -i :4433
netstat -tlnp | grep 4433

# Kill the process or use a different port
kill <PID>
```

### Configuration Error

```
Error: invalid configuration: ...
```

**Solution:**

- Check YAML syntax and indentation
- Verify quotes around values with special characters
- Check CIDR notation is valid
- Ensure required fields are present

### Certificate Not Found

```
Error: open ./certs/agent.crt: no such file or directory
```

**Solution:**

```bash
# Generate certificates
muti-metroo cert ca --cn "My CA" -o ./certs
muti-metroo cert agent --cn "agent-1" \
  --ca ./certs/ca.crt \
  --ca-key ./certs/ca.key \
  -o ./certs
```

### Permission Denied

```
Error: open ./data/agent_id: permission denied
```

**Solution:**

```bash
# Fix ownership
chown -R $(whoami) ./data

# Or fix permissions
chmod 700 ./data
```

## Connection Issues

### Peer Won't Connect

**Check network connectivity:**

```bash
nc -zv peer-address 4433
ping peer-address
```

**Check firewall:**

```bash
sudo iptables -L -n | grep 4433
```

**Check peer ID:**

```yaml
peers:
  - id: "correct-peer-id..."  # Must match peer's agent_id
```

### TLS Handshake Failed

```
Error: tls: handshake failure
```

**Solutions:**

1. Certificate not signed by trusted CA:
   ```bash
   openssl verify -CAfile ca.crt peer.crt
   ```

2. Certificate expired:
   ```bash
   openssl x509 -enddate -noout -in agent.crt
   ```

3. Wrong hostname/IP in certificate:
   ```bash
   openssl x509 -text -noout -in agent.crt | grep -A1 "Subject Alternative Name"
   ```

## SOCKS5 Issues

### No Route to Host

```
Error: no route to 1.2.3.4
```

**Cause:** No exit agent with matching route.

**Solutions:**

1. Check exit is running and connected:
   ```bash
   curl http://localhost:8080/healthz | jq '.peers, .routes'
   ```

2. Check exit has route configured:
   ```yaml
   exit:
     enabled: true
     routes:
       - "0.0.0.0/0"
   ```

3. Trigger route advertisement:
   ```bash
   curl -X POST http://exit-agent:8080/routes/advertise
   ```

### Connection Timeout

**Causes:**
- Slow network
- Too many hops
- Exit agent overloaded

**Solution:**

```yaml
limits:
  stream_open_timeout: 60s  # Default 30s
```

### Authentication Failed

**Solutions:**

1. Verify username/password
2. Check password hash in config is correct
3. Check for special characters needing escaping

## Shell Issues

### Command Rejected

```
Error: command not in whitelist
```

**Solution:**

```yaml
shell:
  whitelist:
    - bash
    - your-command
```

### Shell Authentication Failed

Verify password and hash match. Generate new hash if needed:

```bash
muti-metroo hash
```

## File Transfer Issues

### Path Not Allowed

```
Error: path not in allowed paths
```

**Solution:**

```yaml
file_transfer:
  allowed_paths:
    - /tmp
    - /your/path
```

### File Too Large

**Solution:**

```yaml
file_transfer:
  max_file_size: 0  # Unlimited
```

## Multi-Agent Deployment

### Start Order

Start agents in the correct order:

1. **Listeners first** - Agents with `listeners:` configured
2. **Dialers second** - Agents with `peers:` configured

### Routes Not Propagating

```bash
# Trigger immediate advertisement on exit agent
curl -X POST http://exit-agent:8080/routes/advertise

# Wait and check routes on ingress
sleep 5
curl http://ingress:8080/healthz | jq '.routes'
```

### Reconnection After Restart

Speed up reconnection:

```yaml
connections:
  reconnect:
    initial_delay: 500ms
    max_delay: 10s
```

## Windows-Specific Issues

### Port 8080 Already in Use

```yaml
http:
  address: ":8083"  # Use different port
```

### Running in Background

```powershell
Start-Process -FilePath "muti-metroo.exe" `
  -ArgumentList "run","-c","config.yaml" `
  -WindowStyle Hidden
```

### Path Issues

Use forward slashes or escaped backslashes:

```yaml
data_dir: "C:/muti-metroo/data"
# OR
data_dir: "C:\\muti-metroo\\data"
```

## Debug Mode

Enable debug logging:

```bash
muti-metroo run -c config.yaml --log-level debug
```

Or in config:

```yaml
agent:
  log_level: "debug"
```

## Verification Checklist

After deploying agents:

```bash
# 1. Check each agent is running
curl http://agent:8080/healthz

# 2. Check peer connections
curl http://agent:8080/healthz | jq '.peer_count'

# 3. Check routes exist
curl http://agent:8080/healthz | jq '.route_count'

# 4. Test SOCKS5 proxy
curl --socks5 127.0.0.1:1080 https://ifconfig.me/ip

# 5. Check all nodes visible
curl http://agent:8080/api/nodes | jq '.nodes | length'
```
