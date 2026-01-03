---
title: File Transfer
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting file transfer" style={{maxWidth: '180px'}} />
</div>

# File Transfer

Upload and download files and directories to/from remote agents.

## Configuration

```yaml
file_transfer:
  enabled: true
  max_file_size: 524288000    # 500 MB default, 0 = unlimited
  allowed_paths:
    - /tmp
    - /home/user/uploads
  password_hash: ""           # bcrypt hash (generate with: muti-metroo hash)
```

:::tip Generate Password Hash
Use the built-in CLI to generate bcrypt hashes: `muti-metroo hash --cost 12`

See [CLI - hash](/cli/hash) for details.
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

## Security

Access control via:

1. **allowed_paths**: Only allow uploads to specific directories
2. **password_hash**: Require password for all transfers
3. **enabled flag**: Disable completely if not needed

## Related

- [CLI - File Transfer](../cli/file-transfer) - CLI reference
- [API - File Transfer](../api/file-transfer) - HTTP API reference
- [Security - Access Control](../security/access-control) - Path restrictions
- [Troubleshooting - Common Issues](../troubleshooting/common-issues) - File transfer issues
