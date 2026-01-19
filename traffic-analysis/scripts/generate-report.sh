#!/bin/bash
# Report generator for Muti Metroo Traffic Analysis Lab
# Parses Suricata outputs and generates a comparative analysis report

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB_DIR="$(dirname "$SCRIPT_DIR")"

cd "$LAB_DIR"

REPORT_FILE="$LAB_DIR/output/reports/analysis_report_$(date +%Y%m%d_%H%M%S).md"

echo "=== Generating Traffic Analysis Report ==="
echo "Output: $REPORT_FILE"
echo ""

# Start report
cat > "$REPORT_FILE" << 'EOF'
# Muti Metroo Traffic Analysis Report

Generated: DATE_PLACEHOLDER

## Overview

This report summarizes the traffic analysis of Muti Metroo mesh networking agent
across three transport protocols: QUIC, HTTP/2, and WebSocket.

## Architecture

```
agent1 (ingress) --> agent2 (transit) --> agent3 (exit) --> target
   :31080 SOCKS5     [Suricata capture]      :8093 API      172.28.0.100
```

## Transport Comparison Matrix

| Transport | Port | Protocol | Detectable Signatures |
|-----------|------|----------|----------------------|
| QUIC | UDP 4433 | QUIC + TLS 1.3 | ALPN: muti-metroo/1 |
| HTTP/2 | TCP 8443 | TLS 1.3 + HTTP/2 | X-Muti-Metroo-Protocol header |
| WebSocket | TCP 8888 | HTTP Upgrade + WS | Sec-WebSocket-Protocol: muti-metroo/1 |

EOF

# Replace date placeholder
TIMESTAMP_ISO=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "s/DATE_PLACEHOLDER/$TIMESTAMP_ISO/g" "$REPORT_FILE"
else
    sed -i "s/DATE_PLACEHOLDER/$TIMESTAMP_ISO/g" "$REPORT_FILE"
fi

# Collect test run summaries
echo "## Test Runs" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

for transport in quic h2 ws; do
    # Find the most recent run for this transport
    LATEST_RUN=$(ls -td "$LAB_DIR/output/reports/${transport}_"* 2>/dev/null | head -1)
    TRANSPORT_UPPER=$(echo "$transport" | tr '[:lower:]' '[:upper:]')

    if [ -n "$LATEST_RUN" ] && [ -d "$LATEST_RUN" ]; then
        echo "### $TRANSPORT_UPPER Transport" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        echo "Run directory: \`$(basename "$LATEST_RUN")\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        # Test results
        if [ -f "$LATEST_RUN/tests.jsonl" ]; then
            echo "#### Test Results" >> "$REPORT_FILE"
            echo "" >> "$REPORT_FILE"
            echo "| Test | Name | Duration (s) |" >> "$REPORT_FILE"
            echo "|------|------|--------------|" >> "$REPORT_FILE"

            while IFS= read -r line; do
                test_num=$(echo "$line" | jq -r '.test')
                test_name=$(echo "$line" | jq -r '.name')
                duration=$(echo "$line" | jq -r '.duration')
                echo "| $test_num | $test_name | $duration |" >> "$REPORT_FILE"
            done < "$LATEST_RUN/tests.jsonl"
            echo "" >> "$REPORT_FILE"
        fi

        # Suricata alerts
        if [ -f "$LATEST_RUN/fast.log" ]; then
            ALERT_COUNT=$(wc -l < "$LATEST_RUN/fast.log" | tr -d ' ')
            echo "#### Suricata Alerts: $ALERT_COUNT" >> "$REPORT_FILE"
            echo "" >> "$REPORT_FILE"
            echo "\`\`\`" >> "$REPORT_FILE"
            head -20 "$LATEST_RUN/fast.log" >> "$REPORT_FILE"
            if [ "$ALERT_COUNT" -gt 20 ]; then
                echo "... ($((ALERT_COUNT - 20)) more alerts)" >> "$REPORT_FILE"
            fi
            echo "\`\`\`" >> "$REPORT_FILE"
            echo "" >> "$REPORT_FILE"
        else
            echo "No Suricata alerts captured for this transport." >> "$REPORT_FILE"
            echo "" >> "$REPORT_FILE"
        fi
    else
        echo "### $TRANSPORT_UPPER Transport" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        echo "No test runs found. Run \`./scripts/run-tests.sh $transport\` first." >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    fi
done

# Aggregate Suricata stats from current logs
echo "## Current Suricata Statistics" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

if [ -f "$LAB_DIR/output/logs/suricata/eve.json" ]; then
    echo "### Alert Distribution by Signature" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo "| Signature | Count |" >> "$REPORT_FILE"
    echo "|-----------|-------|" >> "$REPORT_FILE"

    # Extract and count alert signatures
    grep '"event_type":"alert"' "$LAB_DIR/output/logs/suricata/eve.json" 2>/dev/null | \
        jq -r '.alert.signature' 2>/dev/null | \
        sort | uniq -c | sort -rn | head -20 | \
        while read count sig; do
            echo "| $sig | $count |" >> "$REPORT_FILE"
        done

    echo "" >> "$REPORT_FILE"

    # Flow statistics
    echo "### Flow Statistics" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"

    FLOW_COUNT=$(grep '"event_type":"flow"' "$LAB_DIR/output/logs/suricata/eve.json" 2>/dev/null | wc -l | tr -d ' ')
    echo "- Total flows captured: $FLOW_COUNT" >> "$REPORT_FILE"

    # Protocol breakdown
    echo "- Protocol breakdown:" >> "$REPORT_FILE"
    grep '"event_type":"flow"' "$LAB_DIR/output/logs/suricata/eve.json" 2>/dev/null | \
        jq -r '.proto' 2>/dev/null | sort | uniq -c | sort -rn | \
        while read count proto; do
            echo "  - $proto: $count" >> "$REPORT_FILE"
        done

    echo "" >> "$REPORT_FILE"
else
    echo "No EVE JSON log found. Start Suricata and run tests first." >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# Detection effectiveness analysis
cat >> "$REPORT_FILE" << 'EOF'
## Detection Analysis

### Detectable Patterns

| Pattern Type | QUIC | HTTP/2 | WebSocket | Notes |
|--------------|------|--------|-----------|-------|
| ALPN Identifier | Yes | Yes | N/A | "muti-metroo/1" in TLS handshake |
| Protocol Header | N/A | Yes | Yes | X-Muti-Metroo-Protocol / Sec-WebSocket-Protocol |
| Default Ports | 4433/udp | 8443/tcp | 8888/tcp | Configurable |
| Connection Pattern | Long-lived | Long-lived | Long-lived | Persistent mesh links |
| Keepalive Timing | ~5min | ~5min | ~5min | With 20% jitter |

### Not Detectable (E2E Encrypted)

The following cannot be detected by transit traffic analysis due to end-to-end encryption:

- **Payload Content**: ChaCha20-Poly1305 encrypted
- **Frame Types**: Stream open, data, close are encrypted
- **Stream IDs**: Multiplexing is opaque to transit
- **Actual Destinations**: Only exit agent sees real target IPs
- **Shell Commands**: Fully encrypted command execution
- **File Contents**: Transfer data is encrypted

### Behavioral Indicators

Even with E2E encryption, these patterns may be detectable:

1. **Packet Timing**: Regular keepalive intervals (configurable)
2. **Burst Patterns**: Large file transfers create distinctive bursts
3. **Session Duration**: Mesh connections are persistent
4. **Multiplexing**: Multiple logical streams over single connection

## Recommendations

### For Detection (Blue Team)

1. Monitor for custom ALPN values in TLS handshakes
2. Look for WebSocket upgrades with custom subprotocols
3. Track long-lived connections on mesh ports
4. Correlate traffic patterns across nodes

### For Reducing Detectability (Red Team)

1. Use standard ALPN values (h2, http/1.1)
2. Disable custom protocol headers
3. Use common ports (443, 8080)
4. Randomize keepalive timing

## Appendix: Suricata Rules

See `configs/suricata/rules/muti-metroo.rules` for complete rule definitions.

---
*Report generated by Muti Metroo Traffic Analysis Lab*
EOF

echo ""
echo "=== Report Generated ==="
echo "File: $REPORT_FILE"
echo ""

# Also create a summary to stdout
echo "=== Quick Summary ==="
if [ -f "$LAB_DIR/output/logs/suricata/fast.log" ]; then
    TOTAL_ALERTS=$(wc -l < "$LAB_DIR/output/logs/suricata/fast.log" | tr -d ' ')
    echo "Total Suricata alerts: $TOTAL_ALERTS"

    echo ""
    echo "Alert breakdown:"
    grep -oE '\[.*\]' "$LAB_DIR/output/logs/suricata/fast.log" 2>/dev/null | sort | uniq -c | sort -rn | head -10
else
    echo "No Suricata alerts found. Run tests first."
fi
