---
title: File Transfer Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole transferring files" style={{maxWidth: '180px'}} />
</div>

# File Transfer Endpoints

Move files to and from any agent in your mesh. Upload configs, download logs, or transfer entire directories.

**Quick examples:**
```bash
# Upload a file
curl -X POST http://localhost:8080/agents/abc123/file/upload \
  -F "file=@./config.yaml" \
  -F "path=/tmp/config.yaml"

# Download a file
curl -X POST http://localhost:8080/agents/abc123/file/download \
  -d '{"path":"/var/log/app.log"}' -o app.log
```

## POST /agents/\{agent-id\}/file/upload

Upload file or directory to remote agent.

**Content-Type:** `multipart/form-data`

**Form Fields:**
- `file`: File to upload (required)
- `path`: Remote destination path (required)
- `password`: Authentication password (optional)
- `directory`: "true" if uploading directory tar (optional)
- `rate_limit`: Max transfer speed in bytes/second (optional)
- `offset`: Resume from byte offset (optional)
- `original_size`: Expected file size for resume validation (optional)

**Response:**
```json
{
  "success": true,
  "bytes_written": 1024,
  "remote_path": "/tmp/myfile.txt"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/agents/abc123/file/upload   -F "file=@./data.bin"   -F "path=/tmp/data.bin"   -F "password=secret"
```

## POST /agents/\{agent-id\}/file/download

Download file or directory from remote agent.

**Request:**
```json
{
  "password": "your-password",
  "path": "/tmp/myfile.txt",
  "rate_limit": 1048576,
  "offset": 0,
  "original_size": 0
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `password` | string | No | Authentication password |
| `path` | string | Yes | Remote file path to download |
| `rate_limit` | int64 | No | Max transfer speed in bytes/second (0 = unlimited) |
| `offset` | int64 | No | Resume from byte offset |
| `original_size` | int64 | No | Expected file size for resume validation |

**Response:** Binary file data

**Headers:**
- `Content-Type`: `application/octet-stream` (file) or `application/gzip` (directory)
- `Content-Disposition`: Filename
- `X-File-Mode`: File permissions (octal, e.g., "0644")

**Example:**
```bash
curl -X POST http://localhost:8080/agents/abc123/file/download   -H "Content-Type: application/json"   -d '{"password":"secret","path":"/tmp/data.bin"}'   -o data.bin
```

## Implementation Notes

- Files are streamed in 16KB chunks
- No inherent size limits
- Directories are automatically tar/gzip compressed
- File permissions are preserved

### Rate Limiting

When `rate_limit` is set, the sending agent throttles the transfer to the specified bytes per second. The rate limiter uses a token bucket algorithm with a 16KB burst size.

### Resume Transfers

To resume an interrupted transfer:

1. Set `offset` to the number of bytes already received
2. Set `original_size` to the expected total file size

The remote agent validates that the file size matches `original_size`. If the file was modified (size changed), the transfer fails with error code `ErrResumeFailed` (19).

Resume is not supported for directory transfers (tar archives).

## Security

- Requires `file_transfer.enabled: true`
- Optional password authentication via `file_transfer.password_hash`
- Path restrictions via `file_transfer.allowed_paths`

See [File Transfer Feature](/features/file-transfer) for configuration.
