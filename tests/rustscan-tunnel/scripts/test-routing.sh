#!/bin/bash
# Verify routing through TUN interface
set -e

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
echo "Route to kreata.ee (178.33.49.65):"
ip route get 178.33.49.65 || echo "No route"

echo ""
echo "Route to dns-02.emailengine.app (162.19.67.61):"
ip route get 162.19.67.61 || echo "No route"

echo ""
echo "=== Testing TCP connectivity through tunnel ==="
echo "Testing kreata.ee:80..."
if timeout 10 nc -zv 178.33.49.65 80 2>&1; then
    echo "SUCCESS: kreata.ee:80 reachable"
else
    echo "FAILED: kreata.ee:80 not reachable"
fi

echo ""
echo "Testing dns-02.emailengine.app:80..."
if timeout 10 nc -zv 162.19.67.61 80 2>&1; then
    echo "SUCCESS: dns-02.emailengine.app:80 reachable"
else
    echo "FAILED: dns-02.emailengine.app:80 not reachable"
fi

echo ""
echo "=== Quick nmap TCP scan to verify tunnel ==="
echo "Scanning kreata.ee common ports..."
nmap -sT -Pn -p 22,80,443 --open 178.33.49.65

echo ""
echo "Scanning dns-02.emailengine.app common ports..."
nmap -sT -Pn -p 22,80,443 --open 162.19.67.61

echo ""
echo "=== UDP scan (note: shows open|filtered through tunnel) ==="
nmap -sU -Pn -p 53 162.19.67.61
