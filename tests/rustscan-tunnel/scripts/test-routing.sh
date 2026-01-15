#!/bin/bash
# Verify routing through TUN interface
set -e

TARGET_IP="192.168.107.100"

echo "=== Checking TUN interface ==="
if ip addr show tun0 2>/dev/null; then
    ip addr show tun0
else
    echo "ERROR: TUN interface not found"
    echo "Run /scripts/start-client.sh first"
    exit 1
fi

echo ""
echo "=== Checking route table ==="
echo "Route to target ($TARGET_IP):"
ip route get $TARGET_IP || echo "No route"

echo ""
echo "=== Testing TCP connectivity through tunnel ==="
echo "Testing target:80..."
if timeout 10 nc -zv $TARGET_IP 80 2>&1; then
    echo "SUCCESS: target:80 reachable"
else
    echo "FAILED: target:80 not reachable"
fi

echo ""
echo "Testing target:22..."
if timeout 10 nc -zv $TARGET_IP 22 2>&1; then
    echo "SUCCESS: target:22 reachable"
else
    echo "FAILED: target:22 not reachable"
fi

echo ""
echo "=== Quick nmap TCP scan to verify tunnel ==="
echo "Scanning target common ports..."
nmap -sT -Pn -p 22,80,443 --open $TARGET_IP

echo ""
echo "=== UDP scan (note: shows open|filtered through tunnel) ==="
nmap -sU -Pn -p 53,123 $TARGET_IP

echo ""
echo "=== RustScan TCP test ==="
rustscan -a $TARGET_IP -r 1-500 --ulimit 5000 --greppable

echo ""
echo "=== RustScan UDP test ==="
rustscan -a $TARGET_IP --udp -r 53-53 --ulimit 5000 --greppable
