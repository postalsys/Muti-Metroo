---
title: Streams
sidebar_position: 5
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-escalator.png" alt="Mole on data flow" style={{maxWidth: '180px'}} />
</div>

# Stream Multiplexing

Muti Metroo uses virtual streams to multiplex multiple connections over a single peer connection.

## Overview

A **stream** represents a single TCP-like connection flowing through the mesh. Streams are:

- **Multiplexed**: Many streams share one peer connection
- **Bidirectional**: Data flows both directions
- **Encrypted**: End-to-end encryption between ingress and exit
- **Buffered**: Each hop buffers data for flow control
- **Half-closable**: Can close one direction independently

## How Streams Work

When you connect through the SOCKS5 proxy:

1. Your application connects to the SOCKS5 proxy
2. The ingress agent opens a stream through the mesh
3. The exit agent establishes a TCP connection to the destination
4. Data flows bidirectionally through the stream
5. The stream closes when either side disconnects

Transit agents simply forward data without being able to read it (end-to-end encrypted).

## Buffering

Each stream is buffered at each hop for flow control:

```yaml
limits:
  buffer_size: 262144    # 256 KB per stream per hop
```

When a buffer fills, backpressure propagates upstream, preventing memory exhaustion.

## Stream Limits

Configure stream limits based on your hardware capacity:

```yaml
limits:
  max_streams_per_peer: 1000   # Concurrent streams per peer
  max_streams_total: 10000     # Total concurrent streams
  max_pending_opens: 100       # Pending stream requests
  stream_open_timeout: 30s     # Time to open stream
```

| Limit | Purpose |
|-------|---------|
| max_streams_per_peer | Prevent single peer from exhausting resources |
| max_streams_total | Overall memory and CPU protection |
| max_pending_opens | Prevent connection flood |
| stream_open_timeout | Fail fast on network issues |

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

- [End-to-End Encryption](../security/e2e-encryption) - How stream data is protected
- [Performance Troubleshooting](../troubleshooting/performance) - Optimization tips
