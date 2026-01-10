---
title: run
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole running commands" style={{maxWidth: '180px'}} />
</div>

# muti-metroo run

Start the agent. It reads your config file, connects to peers, and begins accepting connections.

```bash
muti-metroo run -c config.yaml
```

The agent runs in the foreground and logs to stdout. Use `Ctrl+C` to stop. For background operation, see [System Service](/deployment/system-service).

## Usage

```bash
muti-metroo run -c <config-file>
```

## Flags

- `-c, --config <file>`: Path to configuration file (default: `./config.yaml`)

## Examples

```bash
# Run with config file
muti-metroo run -c ./config.yaml

# Run with different config
muti-metroo run -c /etc/muti-metroo/config.yaml
```

## Environment Variables

Config file supports environment variable substitution:

```yaml
agent:
  data_dir: "${DATA_DIR:-./data}"
  log_level: "${LOG_LEVEL:-info}"
```

Set before running:

```bash
export DATA_DIR=/var/lib/muti-metroo
export LOG_LEVEL=debug
muti-metroo run -c config.yaml
```
