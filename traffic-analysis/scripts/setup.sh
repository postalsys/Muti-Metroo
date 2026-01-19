#!/bin/bash
# Setup script for Muti Metroo Traffic Analysis Lab
# Generates fresh certificates and initializes agent data directories

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$LAB_DIR")"

cd "$LAB_DIR"

echo "=== Muti Metroo Traffic Analysis Lab Setup ==="
echo "Lab directory: $LAB_DIR"
echo ""

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "ERROR: Docker is not installed or not in PATH"
    exit 1
fi

# Create directories
echo "=== Creating directories ==="
mkdir -p certs data/agent1 data/agent2 data/agent3 output/pcaps output/logs/suricata output/reports test-files

# Build the muti-metroo image using the lab's docker-compose
echo "=== Building muti-metroo Docker image ==="
docker compose build agent1
IMAGE_NAME=$(docker compose images agent1 --format json | head -1 | grep -o '"Image":"[^"]*"' | cut -d'"' -f4)
if [ -z "$IMAGE_NAME" ]; then
    # Fallback: use the service name pattern
    IMAGE_NAME="traffic-analysis-agent1"
fi
echo "Using image: $IMAGE_NAME"
echo ""

# Generate test files for file transfer tests
echo "=== Generating test files ==="
dd if=/dev/urandom of=test-files/5mb-test.bin bs=1048576 count=5 2>/dev/null || dd if=/dev/urandom of=test-files/5mb-test.bin bs=1m count=5 2>/dev/null
dd if=/dev/urandom of=test-files/10mb-test.bin bs=1048576 count=10 2>/dev/null || dd if=/dev/urandom of=test-files/10mb-test.bin bs=1m count=10 2>/dev/null
echo "Test files created: 5mb-test.bin, 10mb-test.bin"
echo ""

# Generate fresh CA certificate
echo "=== Generating CA certificate ==="
docker run --rm -v "$LAB_DIR/certs:/certs" "$IMAGE_NAME" \
    cert ca --cn "Traffic Analysis CA" -o /certs
echo ""

# Generate agent certificates
echo "=== Generating agent certificates ==="
for i in 1 2 3; do
    echo "Generating certificate for ta-agent$i..."
    docker run --rm -v "$LAB_DIR/certs:/certs" "$IMAGE_NAME" \
        cert agent --cn "ta-agent$i" --ca /certs/ca.crt --ca-key /certs/ca.key -o /certs
done
echo ""

# Initialize agent data directories
echo "=== Initializing agent data directories ==="
for i in 1 2 3; do
    echo "Initializing agent$i..."
    docker run --rm -v "$LAB_DIR/data/agent$i:/app/data" "$IMAGE_NAME" \
        init -d /app/data
done
echo ""

# Extract agent IDs from agent_id files
echo "=== Extracting agent IDs ==="
AGENT1_ID=$(cat "$LAB_DIR/data/agent1/agent_id" | tr -d '\n')
AGENT2_ID=$(cat "$LAB_DIR/data/agent2/agent_id" | tr -d '\n')
AGENT3_ID=$(cat "$LAB_DIR/data/agent3/agent_id" | tr -d '\n')

echo "Agent1 ID: $AGENT1_ID"
echo "Agent2 ID: $AGENT2_ID"
echo "Agent3 ID: $AGENT3_ID"
echo ""

# Update config files with actual agent IDs
echo "=== Updating configuration files ==="

# Agent1 config - needs agent2 ID for peer
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS sed requires empty string after -i
    sed -i '' "s/\${AGENT2_ID}/$AGENT2_ID/g" "$LAB_DIR/configs/agent1.yaml"
    sed -i '' "s/\${AGENT3_ID}/$AGENT3_ID/g" "$LAB_DIR/configs/agent2.yaml"
else
    # GNU sed
    sed -i "s/\${AGENT2_ID}/$AGENT2_ID/g" "$LAB_DIR/configs/agent1.yaml"
    sed -i "s/\${AGENT3_ID}/$AGENT3_ID/g" "$LAB_DIR/configs/agent2.yaml"
fi
echo "Updated agent1.yaml with agent2 ID"
echo "Updated agent2.yaml with agent3 ID"

echo ""

# Save agent IDs to a file for scripts to use
cat > "$LAB_DIR/data/agent-ids.env" << EOF
AGENT1_ID=$AGENT1_ID
AGENT2_ID=$AGENT2_ID
AGENT3_ID=$AGENT3_ID
EOF
echo "Agent IDs saved to data/agent-ids.env"

# Create classification.config for Suricata (required)
echo "=== Creating Suricata classification config ==="
cat > "$LAB_DIR/configs/suricata/classification.config" << 'EOF'
# Suricata classification.config
config classification: misc-activity,Misc activity,3
config classification: bad-unknown,Potentially Bad Traffic,2
config classification: attempted-recon,Attempted Information Leak,2
config classification: successful-recon-limited,Information Leak,2
config classification: policy-violation,Potential Corporate Privacy Violation,1
EOF

# Create reference.config for Suricata (required)
cat > "$LAB_DIR/configs/suricata/reference.config" << 'EOF'
# Suricata reference.config
config reference: muti-metroo https://mutimetroo.com
EOF

echo ""
echo "=== Setup Complete ==="
echo ""
echo "To start the lab:"
echo "  cd $LAB_DIR"
echo "  docker compose up -d"
echo ""
echo "To run tests:"
echo "  ./scripts/run-tests.sh quic   # Test QUIC transport"
echo "  ./scripts/run-tests.sh h2     # Test HTTP/2 transport"
echo "  ./scripts/run-tests.sh ws     # Test WebSocket transport"
echo ""
echo "To generate report:"
echo "  ./scripts/generate-report.sh"
