# Windows Event Log Analysis Lab

Lab environment for analyzing Windows event logs when Muti Metroo operates in different roles (transit vs exit).

## Network Topology

```
Agent 1 (Docker)          Agent 2 (Windows)         Agent 3 (Docker)
SOCKS5 Ingress    ---->   Transit/Exit      <----   Exit Node
localhost:1080            10.8.0.8:4433             0.0.0.0/0
                          Exit: 178.33.49.65/32
                          Shell + File Transfer
```

- **Agent 1**: SOCKS5 entry point, dials Windows agent
- **Agent 2**: Windows listener, acts as transit AND exit (for 178.33.49.65)
- **Agent 3**: Default route exit node, dials Windows agent

## Setup

### 1. Windows Agent Preparation

On the Windows machine (10.8.0.8):

```powershell
# Create directory
mkdir C:\muti-metroo
cd C:\muti-metroo

# Download binary from GitHub releases
# https://github.com/postalsys/Muti-Metroo/releases

# Initialize identity
.\muti-metroo.exe init -d .\data

# Generate password hash
.\muti-metroo.exe hash "test123"
# Copy the hash output
```

Copy `configs/agent2-windows.yaml` to `C:\muti-metroo\config.yaml` and replace:
- `${SHELL_PASSWORD_HASH}` with the generated hash
- `${FILE_TRANSFER_PASSWORD_HASH}` with the same hash

### 2. Enable Windows Auditing (for detection research)

Run as Administrator in PowerShell:

```powershell
# Enable Process Creation auditing
auditpol /set /subcategory:"Process Creation" /success:enable /failure:enable

# Enable command line in process creation events
reg add "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System\Audit" /v ProcessCreationIncludeCmdLine_Enabled /t REG_DWORD /d 1 /f

# Enable PowerShell Script Block Logging
reg add "HKLM\SOFTWARE\Policies\Microsoft\Windows\PowerShell\ScriptBlockLogging" /v EnableScriptBlockLogging /t REG_DWORD /d 1 /f

# Enable Windows Firewall logging
netsh advfirewall set allprofiles logging allowedconnections enable
netsh advfirewall set allprofiles logging droppedconnections enable
```

### 3. Docker Agents

```bash
# Generate password hash
./build/muti-metroo hash "test123"

# Create .env file
cp .env.example .env
# Edit .env with the hash

# Start Docker agents
docker compose up -d

# Check status
docker compose logs -f
```

### 4. Start Windows Agent

```powershell
cd C:\muti-metroo
.\muti-metroo.exe run -c .\config.yaml
```

## Test Scenarios

### Get Windows Agent ID

```bash
# From Agent 1's API
curl -s http://localhost:8081/agents | jq '.agents[] | select(.display_name == "Windows-Transit")'
```

### Scenario 1: Windows as Transit Only

Traffic flows: Client -> Agent1 -> Windows -> Agent3 -> Internet

```bash
# Simple request
curl -x socks5h://localhost:1080 https://httpbin.org/ip

# Bulk transfer
curl -x socks5h://localhost:1080 -o /dev/null https://speed.hetzner.de/100MB.bin
```

### Scenario 2: Windows as Exit

Traffic flows: Client -> Agent1 -> Windows -> kreata.ee (178.33.49.65)

```bash
# HTTP request
curl -x socks5h://localhost:1080 http://178.33.49.65:80

# HTTPS request
curl -x socks5h://localhost:1080 https://kreata.ee
```

### Scenario 3: Remote Shell on Windows

```bash
# Replace <win-id> with the Windows agent ID

# Basic commands
muti-metroo shell -p test123 <win-id> whoami
muti-metroo shell -p test123 <win-id> systeminfo
muti-metroo shell -p test123 <win-id> netstat -an
muti-metroo shell -p test123 <win-id> tasklist

# PowerShell
muti-metroo shell -p test123 <win-id> powershell -Command "Get-EventLog -LogName System -Newest 100"

# Registry access
muti-metroo shell -p test123 <win-id> reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion"

# Service queries
muti-metroo shell -p test123 <win-id> sc query
muti-metroo shell -p test123 <win-id> wmic os get caption,version

# Interactive PTY
muti-metroo shell --tty -p test123 <win-id> cmd.exe
```

### Scenario 4: File Transfer

```bash
# Download Windows event log
muti-metroo download -p test123 <win-id> "C:\Windows\System32\winevt\Logs\Security.evtx" ./Security.evtx

# Upload test file
echo "test content" > test.txt
muti-metroo upload -p test123 <win-id> ./test.txt "C:\muti-metroo\test.txt"
```

## Log Collection

### Export Event Logs from Windows

Via remote shell:

```bash
# Export Security log
muti-metroo shell -p test123 <win-id> wevtutil epl Security C:\muti-metroo\Security.evtx

# Export System log
muti-metroo shell -p test123 <win-id> wevtutil epl System C:\muti-metroo\System.evtx

# Export Application log
muti-metroo shell -p test123 <win-id> wevtutil epl Application C:\muti-metroo\Application.evtx

# Export PowerShell log
muti-metroo shell -p test123 <win-id> wevtutil epl "Microsoft-Windows-PowerShell/Operational" C:\muti-metroo\PowerShell.evtx

# Export Sysmon log (if installed)
muti-metroo shell -p test123 <win-id> wevtutil epl "Microsoft-Windows-Sysmon/Operational" C:\muti-metroo\Sysmon.evtx
```

### Download Event Logs

```bash
# Download all exported logs
for log in Security System Application PowerShell Sysmon; do
  muti-metroo download -p test123 <win-id> "C:\muti-metroo\${log}.evtx" ./logs/${log}.evtx
done
```

## Key Event IDs to Analyze

### Standard Windows Events

| Event ID | Log | Description |
|----------|-----|-------------|
| 4688 | Security | Process Creation (agent startup, shell commands) |
| 5156 | Security | WFP Connection (network connections) |
| 7045 | System | Service Install |

### Sysmon Events

| Event ID | Description |
|----------|-------------|
| 1 | Process Create (full command line with hashes) |
| 3 | Network Connection (source/dest IPs and ports) |
| 11 | FileCreate (data directory, transferred files) |
| 22 | DNS Query (exit node DNS resolution) |

## Cleanup

```bash
# Stop Docker agents
docker compose down

# Remove data directories
rm -rf data/

# On Windows: stop the agent (Ctrl+C) and optionally remove C:\muti-metroo
```
