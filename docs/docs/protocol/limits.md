---
title: Limits and Performance
---

# Limits and Performance Characteristics

## Configurable Limits

| Parameter | Config Key | Default | Description |
|-----------|------------|---------|-------------|
| Max Hops | `routing.max_hops` | 16 | Maximum hops for route advertisements |
| Route TTL | `routing.route_ttl` | 5m | Route expiration time |
| Advertise Interval | `routing.advertise_interval` | 2m | Route advertisement frequency |
| Node Info Interval | `routing.node_info_interval` | 2m | Node info frequency |
| Stream Open Timeout | `limits.stream_open_timeout` | 30s | Stream open round-trip time |
| Buffer Size | `limits.buffer_size` | 256 KB | Per-stream buffer at each hop |
| Max Streams/Peer | `limits.max_streams_per_peer` | 1000 | Concurrent streams per peer |
| Max Total Streams | `limits.max_streams_total` | 10000 | Total concurrent streams |
| Max Pending Opens | `limits.max_pending_opens` | 100 | Pending stream opens |
| Idle Threshold | `connections.idle_threshold` | 30s | Keepalive interval |
| Connection Timeout | `connections.timeout` | 90s | Keepalive timeout |

## Protocol Constants (Non-configurable)

| Constant | Value | Description |
|----------|-------|-------------|
| Max Frame Payload | 16 KB | Maximum payload per frame |
| Max Frame Size | 16398 bytes | Payload + 14-byte header |
| Header Size | 14 bytes | Frame header size |
| Protocol Version | 0x01 | Current wire protocol version |
| Control Stream ID | 0 | Reserved for control channel |

## Proxy Chain Practical Limits

**Important**: `max_hops` only limits route advertisement propagation, NOT stream path length. Stream paths are limited by the 30-second open timeout.

| Use Case | Recommended Max Hops | Limiting Factor |
|----------|---------------------|-----------------|
| Interactive SSH | 8-12 hops | Latency (5-50ms per hop) |
| Video Streaming | 6-10 hops | Buffering (256KB × hops) |
| Bulk Transfer | 12-16 hops | Throughput (16KB chunks) |
| High-latency WAN | 4-6 hops | 30s stream open timeout |

**Per-hop overhead:**
- **Latency**: +1-5ms (LAN), +50-200ms (WAN)
- **Memory**: +256KB buffer per active stream
- **CPU**: Frame decode/encode at each relay

## Topology Support

The flood-based routing supports arbitrary mesh topologies:

- **Linear chains**: A → B → C → D
- **Tree structures**: A → B → C and A → B → D
- **Full mesh**: Any agent to any agent
- **Redundant paths**: Multiple paths (lowest metric wins)

Loop prevention via `SeenBy` lists in route advertisements.

## Prometheus Metrics

All metrics prefixed with `muti_metroo_`.

### Connection Metrics

- `peers_connected` (gauge): Connected peers
- `peers_total` (counter): Total peer connections
- `peer_connections_total` (counter): By transport/direction
- `peer_disconnects_total` (counter): By reason

### Stream Metrics

- `streams_active` (gauge): Active streams
- `streams_opened_total` (counter): Total streams opened
- `streams_closed_total` (counter): Total streams closed
- `stream_open_latency_seconds` (histogram): Open latency
- `stream_errors_total` (counter): By error type

### Data Transfer Metrics

- `bytes_sent_total` (counter): By type
- `bytes_received_total` (counter): By type
- `frames_sent_total` (counter): By frame type
- `frames_received_total` (counter): By frame type

### Routing Metrics

- `routes_total` (gauge): Routes in table
- `route_advertises_total` (counter): Advertisements processed
- `route_withdrawals_total` (counter): Withdrawals processed
- `route_flood_latency_seconds` (histogram): Flood latency

### SOCKS5 Metrics

- `socks5_connections_active` (gauge): Active connections
- `socks5_connections_total` (counter): Total connections
- `socks5_auth_failures_total` (counter): Auth failures
- `socks5_connect_latency_seconds` (histogram): Connect latency

### Exit Handler Metrics

- `exit_connections_active` (gauge): Active exit connections
- `exit_connections_total` (counter): Total exit connections
- `exit_dns_queries_total` (counter): DNS queries
- `exit_dns_latency_seconds` (histogram): DNS latency
- `exit_errors_total` (counter): By error type

### Protocol Metrics

- `handshake_latency_seconds` (histogram): Handshake time
- `handshake_errors_total` (counter): By error type
- `keepalives_sent_total` (counter): Keepalives sent
- `keepalives_received_total` (counter): Keepalives received
- `keepalive_rtt_seconds` (histogram): Round-trip time

### RPC Metrics

- `rpc_calls_total` (counter): By result and command
- `rpc_call_duration_seconds` (histogram): By command
- `rpc_bytes_received_total` (counter): Request bytes
- `rpc_bytes_sent_total` (counter): Response bytes

### File Transfer Metrics

- `file_transfer_uploads_total` (counter): Upload count
- `file_transfer_downloads_total` (counter): Download count
- `file_transfer_bytes_sent` (counter): Bytes sent
- `file_transfer_bytes_received` (counter): Bytes received
