---
title: File Transfer (upload/download)
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-reading.png" alt="Mole transferring files" style={{maxWidth: '180px'}} />
</div>

# File Transfer Commands

Move files and directories to and from remote agents through the mesh. Works with any file size - data streams directly without loading into memory.

**Quick examples:**
```bash
# Upload a file
muti-metroo upload abc123 ./local-file.txt /tmp/remote-file.txt

# Download a file
muti-metroo download abc123 /etc/config.yaml ./config.yaml

# Upload an entire directory
muti-metroo upload abc123 ./my-folder /tmp/my-folder
```

## muti-metroo upload

Upload file or directory to remote agent.

### Usage

```bash
muti-metroo upload [flags] <target-agent-id> <local-path> <remote-path>
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent HTTP API address |
| `--password` | `-p` | | File transfer password |
| `--timeout` | `-t` | `300` | Transfer timeout in seconds |
| `--rate-limit` | | | Max transfer speed (e.g., 100KB, 1MB, 10MiB) |
| `--resume` | | `false` | Resume interrupted transfer if possible |

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

# Rate-limited upload (100 KB/s)
muti-metroo upload --rate-limit 100KB abc123 ./large.iso /tmp/large.iso

# Resume interrupted upload
muti-metroo upload --resume abc123 ./huge.iso /tmp/huge.iso
```

## muti-metroo download

Download file or directory from remote agent.

### Usage

```bash
muti-metroo download [flags] <target-agent-id> <remote-path> <local-path>
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent HTTP API address |
| `--password` | `-p` | | File transfer password |
| `--timeout` | `-t` | `300` | Transfer timeout in seconds |
| `--rate-limit` | | | Max transfer speed (e.g., 100KB, 1MB, 10MiB) |
| `--resume` | | `false` | Resume interrupted transfer if possible |

### Examples

```bash
# Download file
muti-metroo download abc123 /etc/config.yaml ./config.yaml

# Download directory
muti-metroo download abc123 /var/log/app ./logs

# With password
muti-metroo download -p secret abc123 /tmp/data.bin ./data.bin

# Rate-limited download (1 MB/s)
muti-metroo download --rate-limit 1MB abc123 /data/backup.tar.gz ./backup.tar.gz

# Resume interrupted download
muti-metroo download --resume abc123 /data/large.iso ./large.iso

# Combine rate limit and resume
muti-metroo download --rate-limit 500KB --resume abc123 /data/huge.iso ./huge.iso
```

## Implementation Notes

- Directories are automatically tar/gzip compressed
- File permissions are preserved
- Streaming transfer (no size limits)

## See Also

- [File Transfer Feature](/features/file-transfer) - Detailed documentation
