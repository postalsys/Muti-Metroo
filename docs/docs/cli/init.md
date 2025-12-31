---
title: init
---

# muti-metroo init

Initialize agent identity (generates AgentID).

## Usage

```bash
muti-metroo init -d <data-dir>
```

## Flags

- `-d, --data-dir <dir>`: Data directory path (required)

## Examples

```bash
# Initialize in current directory
muti-metroo init -d ./data

# Initialize in /var/lib
muti-metroo init -d /var/lib/muti-metroo
```

## What It Does

1. Creates data directory if it doesn't exist
2. Generates a unique 128-bit AgentID
3. Saves AgentID to `agent_id` file in data directory
4. Outputs the AgentID to stdout

## Output

```
Initialized agent: abc123def456789012345678901234ab
Agent ID saved to: ./data/agent_id
```
