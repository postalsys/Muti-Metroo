---
title: Streams
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-escalator.png" alt="Mole on data flow" style={{maxWidth: '180px'}} />
</div>

# Streams

Each connection through the mesh - every curl request, SSH session, or browser tab - becomes a stream. Hundreds of streams share a single connection between agents, so you don't need a new network connection for each request.

**What this means for you:**
- Open many connections without overwhelming the network
- Each stream is encrypted end-to-end (transit agents can't read your data)
- Streams are independent - one slow connection doesn't block others

## How Streams Work

When you run `curl -x socks5://localhost:1080 https://example.com`:

1. Your curl connects to the SOCKS5 proxy
2. The ingress agent opens a stream through the mesh
3. The exit agent opens a TCP connection to example.com
4. Data flows through the stream in both directions
5. When curl finishes, the stream closes

Transit agents forward the encrypted data without seeing the contents.

## Buffering

Each stream is buffered at each hop for flow control:

```yaml
limits:
  buffer_size: 262144    # 256 KB per stream per hop
```

When a buffer fills, backpressure propagates upstream, preventing memory exhaustion.

## Stream Limits

Control how many concurrent connections your agent handles:

```yaml
limits:
  max_streams_per_peer: 1000   # Max connections through one peer
  max_streams_total: 10000     # Max total connections
  max_pending_opens: 100       # Max connections being established
  stream_open_timeout: 30s     # Give up if connection takes too long
```

| Setting | What It Controls | Default |
|---------|------------------|---------|
| max_streams_per_peer | Connections through any single peer | 1000 |
| max_streams_total | Total connections across all peers | 10000 |
| max_pending_opens | Connections waiting to be established | 100 |
| stream_open_timeout | How long to wait for a connection | 30s |

## Performance Considerations

### Latency

Stream open latency increases with hop count:

| Hops | RTT per hop | Approximate stream open time |
|------|-------------|------------------------------|
| 2 | 50ms | ~100ms |
| 5 | 50ms | ~250ms |

### Memory

Memory usage per stream: `buffer_size x number_of_hops`

| Configuration | Memory per stream | 1000 streams |
|---------------|-------------------|--------------|
| 256 KB buffer, 3 hops | 768 KB | ~750 MB |

## Best Practices

1. **Set appropriate limits**: Match your hardware capacity
2. **Monitor stream counts**: Watch for connection leaks
3. **Use reasonable timeouts**: 30s is usually enough
4. **Size buffers appropriately**: 256 KB is a good default

## Troubleshooting

### Streams Not Opening

```bash
# Check health status
curl http://localhost:8080/healthz | jq

# Enable debug logging
muti-metroo run --log-level debug
```

### High Latency

- Check network RTT between hops
- Verify no congestion (buffer full)
- Check CPU usage at transit nodes

### Memory Issues

- Reduce `max_streams_total`
- Reduce `buffer_size`
- Add more agents to distribute load

## Next Steps

- [End-to-End Encryption](/security/e2e-encryption) - How stream data is protected
- [Performance Troubleshooting](/troubleshooting/performance) - Optimization tips
