---
title: rpc
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole executing RPC" style={{maxWidth: '180px'}} />
</div>

# muti-metroo rpc

Execute remote procedure call on a remote agent.

## Usage

```bash
muti-metroo rpc [-a <agent-addr>] [-p <password>] [-t <timeout>] <target-agent-id> <command> [args...]
```

## Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: RPC password
- `-t, --timeout <seconds>`: Command timeout (default: 60)

## Examples

```bash
# Basic command
muti-metroo rpc abc123 whoami

# With arguments
muti-metroo rpc abc123 ls -la /tmp

# With password
muti-metroo rpc -p secret abc123 hostname

# Via different agent
muti-metroo rpc -a 192.168.1.10:8080 abc123 ip addr

# Pipe stdin
echo "test" | muti-metroo rpc abc123 cat
cat file.txt | muti-metroo rpc abc123 wc -l
```

## Exit Codes

The command exits with the same code as the remote command.

## See Also

- [RPC Feature](../features/rpc) - Detailed RPC documentation
- [Configuration](../configuration/overview) - RPC configuration
