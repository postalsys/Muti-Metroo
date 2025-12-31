---
title: File Transfer (upload/download)
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole transferring files" style={{maxWidth: '180px'}} />
</div>

# File Transfer Commands

Upload and download files to/from remote agents.

## muti-metroo upload

Upload file or directory to remote agent.

### Usage

```bash
muti-metroo upload [-a <agent-addr>] [-p <password>] [-t <timeout>] <target-agent-id> <local-path> <remote-path>
```

### Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: File transfer password
- `-t, --timeout <seconds>`: Transfer timeout (default: 300)

### Examples

```bash
# Upload file
muti-metroo upload abc123 ./data.bin /tmp/data.bin

# Upload directory (auto-detected)
muti-metroo upload abc123 ./mydir /tmp/mydir

# With password
muti-metroo upload -p secret abc123 file.txt /tmp/file.txt

# Via different agent
muti-metroo upload -a 192.168.1.10:8080 abc123 data.txt /tmp/data.txt
```

## muti-metroo download

Download file or directory from remote agent.

### Usage

```bash
muti-metroo download [-a <agent-addr>] [-p <password>] [-t <timeout>] <target-agent-id> <remote-path> <local-path>
```

### Flags

- `-a, --agent <addr>`: Agent HTTP API address (default: localhost:8080)
- `-p, --password <pass>`: File transfer password
- `-t, --timeout <seconds>`: Transfer timeout (default: 300)

### Examples

```bash
# Download file
muti-metroo download abc123 /etc/config.yaml ./config.yaml

# Download directory
muti-metroo download abc123 /var/log/app ./logs

# With password
muti-metroo download -p secret abc123 /tmp/data.bin ./data.bin
```

## Implementation Notes

- Directories are automatically tar/gzip compressed
- File permissions are preserved
- Streaming transfer (no size limits)

## See Also

- [File Transfer Feature](../features/file-transfer) - Detailed documentation
