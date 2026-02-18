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

## POST /agents/\{agent-id\}/file/browse

Browse the filesystem on a remote agent. Supports directory listing, file stat, and discovering browsable root paths. Uses the same `allowed_paths` and `password_hash` configuration as file transfer.

### Action: list

List directory contents with pagination.

**Request:**
```json
{
  "action": "list",
  "path": "/tmp",
  "password": "secret",
  "offset": 0,
  "limit": 100
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | No | `"list"` (default), `"stat"`, or `"roots"` |
| `path` | string | Yes | Directory path to list |
| `password` | string | No | Authentication password |
| `offset` | int | No | Pagination offset (default 0) |
| `limit` | int | No | Max entries to return (default 100, max 200) |

**Response:**
```json
{
  "path": "/tmp",
  "entries": [
    { "name": "subdir", "size": 4096, "mode": "0755", "mod_time": "2026-02-17T08:00:00Z", "is_dir": true },
    { "name": "file.txt", "size": 1024, "mode": "0644", "mod_time": "2026-02-18T10:30:00Z" },
    { "name": "link", "size": 12, "mode": "0777", "mod_time": "2026-02-16T12:00:00Z", "is_symlink": true, "link_target": "/etc/hosts" }
  ],
  "total": 42,
  "truncated": false
}
```

Entries are sorted with directories first, then files, alphabetically by name within each group. Symlinks include `is_symlink` and `link_target` fields, with size and `is_dir` resolved from the symlink target.

### Action: stat

Get info about a single path.

**Request:**
```json
{ "action": "stat", "path": "/tmp/file.txt", "password": "secret" }
```

**Response:**
```json
{
  "path": "/tmp/file.txt",
  "entry": { "name": "file.txt", "size": 1024, "mode": "0644", "mod_time": "2026-02-18T10:30:00Z" }
}
```

### Action: roots

Discover browsable root paths from the `allowed_paths` configuration.

**Request:**
```json
{ "action": "roots", "password": "secret" }
```

**Response:**
```json
{ "roots": ["/tmp", "/data"], "wildcard": false }
```

When `allowed_paths: ["*"]`, the response is `{ "roots": ["/"], "wildcard": true }`. Glob patterns like `/data/**` are normalized to their base directory `/data`.

**Example:**
```bash
# List directory
curl -X POST http://localhost:8080/agents/abc123/file/browse \
  -H "Content-Type: application/json" \
  -d '{"action":"list","path":"/tmp"}'

# Stat a file
curl -X POST http://localhost:8080/agents/abc123/file/browse \
  -H "Content-Type: application/json" \
  -d '{"action":"stat","path":"/tmp/config.yaml"}'

# Get browsable roots
curl -X POST http://localhost:8080/agents/abc123/file/browse \
  -H "Content-Type: application/json" \
  -d '{"action":"roots"}'
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

## See Also

- [File Transfer Feature](/features/file-transfer) - Feature overview
- [CLI - File Transfer](/cli/file-transfer) - CLI reference
- [File Transfer Configuration](/configuration/file-transfer) - Configuration options
