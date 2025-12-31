---
title: Common Issues
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-drilling.png" alt="Mole fixing issues" style={{maxWidth: '180px'}} />
</div>

# Common Issues

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

# Or change config
listeners:
  - address: "0.0.0.0:4434"   # Different port
```

### Configuration Error

```
Error: invalid configuration: ...
```

**Solution:**

```bash
# Validate YAML syntax
yamllint config.yaml

# Check for common issues:
# - Incorrect indentation
# - Missing quotes around values with special chars
# - Invalid CIDR notation
# - Missing required fields
```

### Certificate Not Found

```
Error: open ./certs/agent.crt: no such file or directory
```

**Solution:**

```bash
# Generate certificates
muti-metroo cert ca -n "My CA"
muti-metroo cert agent -n "agent-1"

# Or fix path in config
tls:
  cert: "/absolute/path/to/agent.crt"
  key: "/absolute/path/to/agent.key"
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

# If running as service user
chown -R muti-metroo:muti-metroo /var/lib/muti-metroo
```

## Connection Issues

### Peer Won't Connect

**Symptoms:**
- `peers: 0` in health check
- Logs show connection attempts but no success

**Solutions:**

1. Check network connectivity:
   ```bash
   # Can you reach the peer?
   nc -zv peer-address 4433
   ping peer-address
   ```

2. Check firewall:
   ```bash
   # Is port open?
   sudo iptables -L -n | grep 4433
   ```

3. Check TLS:
   ```bash
   # Verify certificate
   openssl s_client -connect peer-address:4433
   ```

4. Check peer ID:
   ```yaml
   # Is the ID correct?
   peers:
     - id: "correct-peer-id..."  # Must match peer's agent_id
   ```

### Connection Refused

```
Error: connection refused to peer-address:4433
```

**Solutions:**

1. Peer not running or not listening
2. Wrong address/port
3. Firewall blocking connection
4. Wrong transport type

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
       - "0.0.0.0/0"  # Or specific CIDR
   ```

3. Trigger route advertisement:
   ```bash
   curl -X POST http://exit-agent:8080/routes/advertise
   ```

### Connection Timeout

```
Error: connection timeout
```

**Causes:**
- Slow network
- Too many hops
- Exit agent overloaded

**Solutions:**

1. Increase timeout:
   ```yaml
   limits:
     stream_open_timeout: 60s  # Default 30s
   ```

2. Reduce hop count

3. Check exit agent health

### Authentication Failed

```
Error: SOCKS5 authentication failed
```

**Solutions:**

1. Check username/password:
   ```bash
   curl -x socks5://user:password@localhost:1080 https://example.com
   ```

2. Verify password hash in config is correct

3. Check for special characters needing escaping

## Exit Issues

### DNS Resolution Failed

```
Error: DNS lookup failed for example.com
```

**Solutions:**

1. Check DNS servers are configured:
   ```yaml
   exit:
     dns:
       servers:
         - "8.8.8.8:53"
   ```

2. Check DNS servers are reachable from exit host:
   ```bash
   dig @8.8.8.8 example.com
   ```

3. Increase DNS timeout:
   ```yaml
   exit:
     dns:
       timeout: 10s
   ```

### Destination Not Allowed

```
Error: destination not in allowed routes
```

**Solution:** Add route to exit config:

```yaml
exit:
  routes:
    - "0.0.0.0/0"  # Or specific CIDR matching destination
```

## RPC Issues

### Command Rejected

```
Error: command not in whitelist
```

**Solution:** Add command to whitelist:

```yaml
rpc:
  whitelist:
    - whoami
    - your-command  # Add needed command
```

### RPC Authentication Failed

```
Error: RPC authentication failed
```

**Solutions:**

1. Check password:
   ```bash
   muti-metroo rpc -p correct-password agent-id whoami
   ```

2. Verify password hash in config matches

## File Transfer Issues

### Path Not Allowed

```
Error: path not in allowed paths
```

**Solution:** Add path prefix to allowed_paths:

```yaml
file_transfer:
  allowed_paths:
    - /tmp
    - /path/to/your/files
```

### File Too Large

```
Error: file exceeds maximum size
```

**Solution:** Increase max_file_size or use 0 for unlimited:

```yaml
file_transfer:
  max_file_size: 0  # Unlimited
```

## Performance Issues

See [Performance Troubleshooting](performance) for:
- Slow connections
- High latency
- Memory usage
- CPU usage

## Debug Mode

Enable debug logging for detailed diagnostics:

```bash
muti-metroo run -c config.yaml --log-level debug
```

Or in config:

```yaml
agent:
  log_level: "debug"
```

## Getting Help

If these solutions don't work:

1. Check logs with debug level
2. Review the specific troubleshooting guides
3. Check the protocol and limits documentation
4. Search existing issues
5. Open a new issue with:
   - Configuration (redacted)
   - Logs (debug level)
   - Steps to reproduce

## Next Steps

- [Connectivity Troubleshooting](connectivity)
- [Performance Troubleshooting](performance)
- [FAQ](faq)
