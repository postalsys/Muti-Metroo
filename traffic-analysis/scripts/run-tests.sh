#!/bin/bash
# Test runner script for Muti Metroo Traffic Analysis Lab
# Runs various traffic patterns through the mesh for Suricata analysis
#
# Usage: ./run-tests.sh [quic|h2|ws]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB_DIR="$(dirname "$SCRIPT_DIR")"

cd "$LAB_DIR"

# Load agent IDs
if [ -f "$LAB_DIR/data/agent-ids.env" ]; then
    source "$LAB_DIR/data/agent-ids.env"
else
    echo "ERROR: Agent IDs not found. Run setup.sh first."
    exit 1
fi

# Default transport
TRANSPORT="${1:-quic}"

case "$TRANSPORT" in
    quic)
        PEER_PORT="4433"
        PEER_PATH=""
        TRANSPORT_NAME="QUIC"
        ;;
    h2)
        PEER_PORT="8443"
        PEER_PATH="/h2"
        TRANSPORT_NAME="HTTP/2"
        ;;
    ws)
        PEER_PORT="8888"
        PEER_PATH="/mesh"
        TRANSPORT_NAME="WebSocket"
        ;;
    *)
        echo "Usage: $0 [quic|h2|ws]"
        echo "  quic - Test QUIC transport (UDP :4433)"
        echo "  h2   - Test HTTP/2 transport (TCP :8443)"
        echo "  ws   - Test WebSocket transport (TCP :8888)"
        exit 1
        ;;
esac

echo "=== Muti Metroo Traffic Analysis Tests ==="
echo "Transport: $TRANSPORT_NAME"
echo "Agent3 ID: $AGENT3_ID"
echo ""

# Create output directory for this test run
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RUN_DIR="$LAB_DIR/output/reports/${TRANSPORT}_${TIMESTAMP}"
mkdir -p "$RUN_DIR"

echo "Output directory: $RUN_DIR"
echo ""

# Helper function to update peer transport in configs
update_transport() {
    local transport=$1
    local port=$2
    local path=$3

    echo "Updating peer connections to use $transport transport on port $port..."

    # Generate new agent1 config with updated peer
    cat > "$LAB_DIR/configs/agent1.yaml" << EOF
# Agent1: SOCKS5 Ingress for Traffic Analysis Lab
agent:
  display_name: "ta-agent1-ingress"
  data_dir: "/app/data"

tls:
  ca: "/app/certs/ca.crt"
  cert: "/app/certs/ta-agent1.crt"
  key: "/app/certs/ta-agent1.key"

# All three transport listeners for testing
listeners:
  - transport: quic
    address: ":4433"
  - transport: h2
    address: ":8443"
    path: "/h2"
  - transport: ws
    address: ":8888"
    path: "/mesh"

# Peer connection to agent2
peers:
  - id: "$AGENT2_ID"
    transport: $transport
    address: "agent2:$port"
EOF
    if [ -n "$path" ]; then
        echo "    path: \"$path\"" >> "$LAB_DIR/configs/agent1.yaml"
    fi
    cat >> "$LAB_DIR/configs/agent1.yaml" << 'EOF'

socks5:
  enabled: true
  address: ":1080"

http:
  enabled: true
  address: ":8080"
  dashboard: true
EOF

    # Generate new agent2 config with updated peer
    cat > "$LAB_DIR/configs/agent2.yaml" << EOF
# Agent2: Transit Node for Traffic Analysis Lab
# Suricata runs as a sidecar capturing traffic on this node
agent:
  display_name: "ta-agent2-transit"
  data_dir: "/app/data"

tls:
  ca: "/app/certs/ca.crt"
  cert: "/app/certs/ta-agent2.crt"
  key: "/app/certs/ta-agent2.key"

# All three transport listeners for testing
listeners:
  - transport: quic
    address: ":4433"
  - transport: h2
    address: ":8443"
    path: "/h2"
  - transport: ws
    address: ":8888"
    path: "/mesh"

# Peer connection to agent3
peers:
  - id: "$AGENT3_ID"
    transport: $transport
    address: "agent3:$port"
EOF
    if [ -n "$path" ]; then
        echo "    path: \"$path\"" >> "$LAB_DIR/configs/agent2.yaml"
    fi
    cat >> "$LAB_DIR/configs/agent2.yaml" << 'EOF'

http:
  enabled: true
  address: ":8080"
  dashboard: true
EOF
}

# Update transport configuration
update_transport "$TRANSPORT" "$PEER_PORT" "$PEER_PATH"

# Restart agents to apply new transport
echo "=== Restarting agents with $TRANSPORT_NAME transport ==="
docker compose restart agent1 agent2 agent3
sleep 3
# Suricata needs to be restarted after agent2 restart (network namespace dependency)
echo "=== Restarting Suricata ==="
docker compose restart suricata
sleep 3

# Wait for mesh to establish
echo "=== Waiting for mesh connectivity ==="
MAX_WAIT=30
WAIT_COUNT=0
while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    HEALTH=$(curl -s http://localhost:8091/healthz 2>/dev/null || echo "{}")
    if echo "$HEALTH" | grep -q '"peer_count":1'; then
        echo "Mesh established!"
        break
    fi
    echo "Waiting for mesh... ($WAIT_COUNT/$MAX_WAIT)"
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

if [ $WAIT_COUNT -eq $MAX_WAIT ]; then
    echo "WARNING: Mesh may not be fully established"
fi
echo ""

# Function to run a test and log results
run_test() {
    local test_num=$1
    local test_name=$2
    local test_cmd=$3

    echo "=== Test $test_num: $test_name ==="
    echo "Command: $test_cmd"

    START_TIME=$(date +%s)

    # Run the test
    eval "$test_cmd" > "$RUN_DIR/test${test_num}_output.txt" 2>&1 || true

    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo "Duration: ${DURATION}s"
    echo "Output saved to: test${test_num}_output.txt"
    echo ""

    # Log test metadata
    TIMESTAMP_ISO=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "{\"test\": $test_num, \"name\": \"$test_name\", \"transport\": \"$TRANSPORT\", \"duration\": $DURATION, \"timestamp\": \"$TIMESTAMP_ISO\"}" >> "$RUN_DIR/tests.jsonl"
}

# Ensure Suricata is running
echo "=== Verifying Suricata is capturing ==="
docker compose logs suricata 2>&1 | tail -5
echo ""

# Test 1: Simple HTTP request via SOCKS5
run_test 1 "Simple HTTP Request" \
    "curl -x socks5h://localhost:31080 -s -o /dev/null -w '%{http_code}' http://172.28.0.100/"

# Test 2: Large file download (10MB)
run_test 2 "10MB Download" \
    "curl -x socks5h://localhost:31080 -s -o /dev/null http://172.28.0.100/ --max-time 60 || echo 'Download test (nginx returns small response)'"

# Test 3: Shell command - whoami
run_test 3 "Shell: whoami" \
    "curl -s http://localhost:8091/agents/$AGENT3_ID/shell -H 'Connection: Upgrade' -H 'Upgrade: websocket' --max-time 10 || echo 'Shell test requires WebSocket client'"

# Test 4: File upload (5MB) - using HTTP API
run_test 4 "5MB File Upload" \
    "curl -s -X POST http://localhost:8091/agents/$AGENT3_ID/file/upload -F 'file=@test-files/5mb-test.bin' -F 'path=/tmp/upload-test.bin' --max-time 120"

# Test 5: File download (5MB) - using HTTP API
run_test 5 "5MB File Download" \
    "curl -s -X POST http://localhost:8091/agents/$AGENT3_ID/file/download -H 'Content-Type: application/json' -d '{\"path\":\"/tmp/upload-test.bin\"}' -o /dev/null --max-time 120"

# Test 6: Streaming output (simulated with multiple requests)
run_test 6 "Streaming Pattern (30s)" \
    "for i in \$(seq 1 30); do curl -x socks5h://localhost:31080 -s -o /dev/null http://172.28.0.100/; sleep 1; done"

# Test 7: Concurrent requests (10 parallel)
run_test 7 "10 Concurrent Requests" \
    "for i in \$(seq 1 10); do curl -x socks5h://localhost:31080 -s -o /dev/null http://172.28.0.100/ & done; wait"

# Test 8: ICMP ping via exit
run_test 8 "ICMP Ping" \
    "curl -s http://localhost:8091/agents/$AGENT3_ID/icmp --max-time 10 || echo 'Ping test requires WebSocket client'"

# Collect Suricata stats
echo "=== Collecting Suricata Statistics ==="
docker compose exec -T suricata suricatasc -c "dump-counters" > "$RUN_DIR/suricata_counters.json" 2>/dev/null || true

# Copy Suricata logs
echo "=== Copying Suricata Logs ==="
cp "$LAB_DIR/output/logs/suricata/fast.log" "$RUN_DIR/fast.log" 2>/dev/null || true
cp "$LAB_DIR/output/logs/suricata/eve.json" "$RUN_DIR/eve.json" 2>/dev/null || true

# Summary
echo ""
echo "=== Test Run Complete ==="
echo "Transport: $TRANSPORT_NAME"
echo "Results: $RUN_DIR"
echo ""
echo "Files:"
ls -la "$RUN_DIR/"
echo ""
echo "Quick Suricata alert summary:"
if [ -f "$RUN_DIR/fast.log" ]; then
    wc -l "$RUN_DIR/fast.log"
    echo "Last 10 alerts:"
    tail -10 "$RUN_DIR/fast.log"
else
    echo "No alerts captured yet"
fi
