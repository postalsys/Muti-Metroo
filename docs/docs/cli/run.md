---
title: run
---

# muti-metroo run

Run the agent with a configuration file.

## Usage

```bash
muti-metroo run -c <config-file>
```

## Flags

- `-c, --config <file>`: Path to configuration file (required)

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
