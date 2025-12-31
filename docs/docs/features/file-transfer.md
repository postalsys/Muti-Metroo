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
  max_file_size: 0            # 0 = unlimited
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

## Security

Access control via:

1. **allowed_paths**: Only allow uploads to specific directories
2. **password_hash**: Require password for all transfers
3. **enabled flag**: Disable completely if not needed

## Metrics

- `muti_metroo_file_transfer_uploads_total`: Upload count
- `muti_metroo_file_transfer_downloads_total`: Download count
- `muti_metroo_file_transfer_bytes_sent`: Bytes sent
- `muti_metroo_file_transfer_bytes_received`: Bytes received

## Related

- [CLI - File Transfer](../cli/file-transfer) - CLI reference
- [API - File Transfer](../api/file-transfer) - HTTP API reference
- [Security - Access Control](../security/access-control) - Path restrictions
- [Troubleshooting - Common Issues](../troubleshooting/common-issues) - File transfer issues
