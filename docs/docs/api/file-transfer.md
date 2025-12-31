---
title: File Transfer Endpoints
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-wiring.png" alt="Mole transferring files" style={{maxWidth: '180px'}} />
</div>

# File Transfer Endpoints

File upload and download APIs.

## POST /agents/\{agent-id\}/file/upload

Upload file or directory to remote agent.

**Content-Type:** `multipart/form-data`

**Form Fields:**
- `file`: File to upload (required)
- `path`: Remote destination path (required)
- `password`: Authentication password (optional)
- `directory`: "true" if uploading directory tar (optional)

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
  "path": "/tmp/myfile.txt"
}
```

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

## Security

- Requires `file_transfer.enabled: true`
- Optional password authentication via `file_transfer.password_hash`
- Path restrictions via `file_transfer.allowed_paths`

See [File Transfer Feature](../features/file-transfer) for configuration.
