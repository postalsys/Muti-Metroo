#!/bin/bash
# Start Muti Metroo ingress and Mutiauk TUN daemon in the client container
set -e

echo "=== Starting Muti Metroo ingress agent ==="
muti-metroo run -c /etc/muti-metroo/config.yaml &
MM_PID=$!
echo "Muti Metroo started with PID $MM_PID"

# Wait for SOCKS5 to be ready
echo "Waiting for SOCKS5 proxy to be ready..."
for i in {1..30}; do
    if nc -z 127.0.0.1 1080 2>/dev/null; then
        echo "SOCKS5 proxy is ready"
        break
    fi
    sleep 1
done

# Wait for routes to propagate
echo "Waiting for routes to propagate (10s)..."
sleep 10

echo "=== Starting Mutiauk TUN daemon ==="
mutiauk daemon start -c /etc/mutiauk/config.yaml &
MUTIAUK_PID=$!
echo "Mutiauk started with PID $MUTIAUK_PID"

# Wait for TUN interface
echo "Waiting for TUN interface..."
for i in {1..30}; do
    if ip link show tun0 2>/dev/null; then
        echo "TUN interface is ready"
        break
    fi
    sleep 1
done

echo ""
echo "=== Setup complete ==="
echo "TUN interface:"
ip addr show tun0 2>/dev/null || echo "TUN interface not found"
echo ""
echo "Routes to targets:"
ip route get 178.33.49.65 2>/dev/null || echo "No route to 178.33.49.65"
ip route get 162.19.67.61 2>/dev/null || echo "No route to 162.19.67.61"
echo ""
echo "You can now run RustScan:"
echo "  rustscan -a 178.33.49.65,162.19.67.61 -r 1-1000 --ulimit 5000"
echo ""

# Keep running and drop to shell
exec /bin/bash
