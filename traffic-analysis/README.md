# Muti Metroo Traffic Analysis Lab

A Docker-based traffic analysis lab with Suricata IPS to analyze Muti Metroo protocol patterns across all three transport types.

## Architecture

```
agent1 (ingress) ----> agent2 (transit) ----> agent3 (exit) --> target (nginx)
   :31080 SOCKS5        [Suricata sidecar]       :8093 API      172.28.0.100
   :8091 API                :8092 API            shell/file enabled

Transport Options (tested separately):
  - QUIC:      UDP :4433
  - HTTP/2:    TCP :8443
  - WebSocket: TCP :8888 (path: /mesh)
```

## Quick Start

```bash
# 1. Run setup (generates fresh certs and initializes agents)
./scripts/setup.sh

# 2. Start the lab
docker compose up -d

# 3. Verify mesh connectivity
curl http://localhost:8091/healthz

# 4. Run tests for each transport
./scripts/run-tests.sh quic
./scripts/run-tests.sh h2
./scripts/run-tests.sh ws

# 5. Generate analysis report
./scripts/generate-report.sh
```

## Directory Structure

```
traffic-analysis/
  docker-compose.yml          # Lab services
  certs/                      # Fresh CA and agent certs (generated)
  configs/
    agent1.yaml               # Ingress config
    agent2.yaml               # Transit config
    agent3.yaml               # Exit config (shell + file enabled)
    suricata/
      suricata.yaml           # Suricata configuration
      rules/
        muti-metroo.rules     # Detection signatures
  data/                       # Agent state (generated via init)
  output/
    pcaps/                    # Captured traffic
    logs/suricata/            # Suricata logs
    reports/                  # Generated reports
  test-files/                 # Test data for transfers
  scripts/
    setup.sh                  # One-time setup
    run-tests.sh              # Test runner
    generate-report.sh        # Report generator
```

## Transport Detection Signatures

| Transport | Port | Detectable Signature |
|-----------|------|---------------------|
| QUIC | UDP 4433 | ALPN: muti-metroo/1 in QUIC handshake |
| HTTP/2 | TCP 8443 | X-Muti-Metroo-Protocol HTTP header |
| WebSocket | TCP 8888 | Sec-WebSocket-Protocol: muti-metroo/1 |

## Test Cases

| Test | Activity | Traffic Pattern |
|------|----------|-----------------|
| 1 | curl via SOCKS5 | Short burst, TCP dial at exit |
| 2 | 10MB download | Sustained high bandwidth |
| 3 | Shell: whoami | Small bidirectional WebSocket |
| 4 | 5MB file upload | Unidirectional burst |
| 5 | 5MB file download | Unidirectional burst |
| 6 | tail -f streaming | Continuous small packets (30s) |
| 7 | 10 concurrent requests | Parallel streams |
| 8 | ICMP ping via exit | Ping request/response pattern |

## Viewing Results

### Suricata Alerts

```bash
# Real-time alerts
tail -f output/logs/suricata/fast.log

# Structured JSON logs
jq '.' output/logs/suricata/eve.json | head -100
```

### PCAP Analysis

```bash
# View captured traffic with tshark
tshark -r output/pcaps/capture.pcap

# Filter for specific transport
tshark -r output/pcaps/capture.pcap -Y "udp.port == 4433"  # QUIC
tshark -r output/pcaps/capture.pcap -Y "tcp.port == 8443"  # H2
tshark -r output/pcaps/capture.pcap -Y "tcp.port == 8888"  # WS
```

## What Suricata Can Detect

**Detectable:**
- Protocol identifiers (ALPN, HTTP headers, WS subprotocol)
- Connection patterns (long-lived, mesh topology)
- Keepalive timing (~5min intervals with jitter)
- Traffic bursts (file transfers)
- Multiplexing patterns

**Not Detectable (E2E Encrypted):**
- Payload content (ChaCha20-Poly1305)
- Frame types and stream IDs
- Shell commands and file contents
- Actual destination IPs (only visible at exit)

## Cleanup

```bash
docker compose down -v
rm -rf certs/ data/ output/
```
