---
title: File Transfer
sidebar_position: 4
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting file transfer" style={{maxWidth: '180px'}} />
</div>

# File Transfer

Move files to and from any agent in your mesh. Grab a config file from a remote server, deploy scripts to multiple machines, or exfiltrate data through your tunnel.

```bash
# Download a file from a remote agent
muti-metroo download abc123 /etc/passwd ./passwd.txt

# Upload a script to a remote agent
muti-metroo upload abc123 ./deploy.sh /tmp/deploy.sh

# Transfer entire directories
muti-metroo upload abc123 ./tools /opt/tools
```

:::tip Configuration
See [File Transfer Configuration](/configuration/file-transfer) for all options including path restrictions, size limits, and authentication.
:::

## CLI Usage

### Upload File

```bash
muti-metroo upload <agent-id> <local-path> <remote-path>

# Example
muti-metroo upload abc123 ./data.bin /tmp/data.bin
```

### Upload Directory

```bash
muti-metroo upload abc123 ./mydir /tmp/mydir
```

### Download File

```bash
muti-metroo download <agent-id> <remote-path> <local-path>

# Example
muti-metroo download abc123 /etc/config.yaml ./config.yaml
```

### With Authentication

```bash
muti-metroo upload -p password abc123 ./file.txt /tmp/file.txt
```

## HTTP API

### Upload

`POST /agents/{agent-id}/file/upload`

Multipart form data:
- `file`: File to upload
- `path`: Remote destination path
- `password`: Auth password (optional)
- `directory`: "true" if directory tar

### Download

`POST /agents/{agent-id}/file/download`

Request:
```json
{
  "password": "secret",
  "path": "/tmp/file.txt"
}
```

Response: Binary file data

## Implementation Details

- **Streaming**: Files transferred in 16KB chunks
- **No size limits**: Stream directly without memory buffering
- **Directories**: Automatically tar/gzip with permission preservation
- **Authentication**: bcrypt password hashing
- **Permissions**: File mode preserved (Unix)

## Rate Limiting

Limit transfer speed to avoid saturating network links.

```bash
# Upload at max 100 KB/s
muti-metroo upload --rate-limit 100KB abc123 ./large.iso /tmp/large.iso

# Download at max 1 MB/s
muti-metroo download --rate-limit 1MB abc123 /data/backup.tar.gz ./backup.tar.gz
```

Supported size formats:
- Decimal: `100KB`, `1MB`, `10GB` (1 KB = 1000 bytes)
- Binary: `100KiB`, `1MiB`, `10GiB` (1 KiB = 1024 bytes)
- Plain bytes: `1024000`

Rate limiting is applied by the **sending agent**:
- Upload: Your local agent limits the upload speed
- Download: The remote agent limits the download speed

## Resume Support

Continue interrupted transfers without starting over.

```bash
# Resume interrupted download
muti-metroo download --resume abc123 /data/large.iso ./large.iso

# Resume interrupted upload
muti-metroo upload --resume abc123 ./huge.iso /tmp/huge.iso

# Combine with rate limiting
muti-metroo download --rate-limit 500KB --resume abc123 /data/huge.iso ./huge.iso
```

### How It Works

1. Partial data is written to `<filename>.partial`
2. Progress is tracked in `<filename>.partial.json`
3. On resume, transfer continues from the last byte written
4. On completion, `.partial` is renamed to the final filename

### Validation

Resume uses **file size comparison** to detect if the source file changed:
- If the original file size matches, transfer resumes from the offset
- If size differs, transfer restarts from the beginning

:::note Directory Transfers
Resume is not supported for directory transfers. If a directory transfer is interrupted, it will restart from the beginning.
:::

## Troubleshooting

### Permission Denied

```
Error: path not allowed: /etc/passwd
```

**Cause:** The path is not in the agent's `allowed_paths` configuration.

**Solutions:**
- Check the agent's `allowed_paths` setting
- Use a path that's explicitly allowed
- Contact the agent administrator to add the path

### Authentication Failed

```
Error: authentication required
```

**Cause:** The agent requires password authentication for file transfers.

**Solution:** Provide the password with `-p`:
```bash
muti-metroo upload -p mypassword abc123 ./file.txt /tmp/file.txt
```

### File Too Large

```
Error: file exceeds maximum size limit
```

**Cause:** The file exceeds the agent's `max_file_size` setting.

**Solutions:**
- Check the size limit: `max_file_size: 0` means unlimited
- Split the file into smaller parts
- Contact the agent administrator to increase the limit

### Agent Not Reachable

```
Error: no route to agent abc123
```

**Cause:** The target agent is not connected to the mesh or routes haven't propagated.

**Solutions:**
```bash
# Check if agent is known
curl http://localhost:8080/agents

# Verify connectivity
curl http://localhost:8080/healthz | jq '.peers'
```

### Transfer Interrupted

If a transfer is interrupted, use `--resume` to continue:

```bash
# Resume from where it left off
muti-metroo download --resume abc123 /data/large.iso ./large.iso
```

Note: Resume is not supported for directory transfers.

## Related

- [CLI - File Transfer](/cli/file-transfer) - CLI reference
- [API - File Transfer](/api/file-transfer) - HTTP API reference
- [Security - Access Control](/security/access-control) - Path restrictions
- [Troubleshooting - Common Issues](/troubleshooting/common-issues) - File transfer issues
