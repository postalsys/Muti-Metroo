---
title: File Transfer Configuration
sidebar_position: 13
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole configuring file transfer" style={{maxWidth: '180px'}} />
</div>

# File Transfer Configuration

Upload and download files to remote agents through the mesh. Supports individual files and directories with automatic compression.

:::warning Security Feature
File transfer is disabled by default. Enable only on agents that need file operations, and always use password authentication with path restrictions.
:::

**Minimal secure setup:**
```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$..."  # Generate with: muti-metroo hash
  allowed_paths:
    - /tmp
```

## Configuration

```yaml
file_transfer:
  enabled: false           # Disabled by default
  password_hash: ""        # bcrypt hash of password (required when enabled)
  max_file_size: 524288000 # 500 MB (0 = unlimited)
  allowed_paths: []        # Paths allowed for transfer (empty = none)
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable file transfer |
| `password_hash` | string | `""` | bcrypt hash of authentication password |
| `max_file_size` | int | `524288000` | Maximum file size in bytes (500 MB) |
| `allowed_paths` | list | `[]` | Allowed path patterns |

## Password Authentication

File transfer requires password authentication. Generate a password hash:

```bash
# Interactive (recommended)
muti-metroo hash

# From argument
muti-metroo hash "your-secure-password"
```

Use the generated hash in config:

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
```

## Path Restrictions

The `allowed_paths` list controls which paths can be accessed:

### No Paths (Default)

```yaml
file_transfer:
  allowed_paths: []  # No paths allowed
```

### Specific Directories

```yaml
file_transfer:
  allowed_paths:
    - /tmp           # /tmp and everything under it
    - /var/log       # /var/log and everything under it
    - /home/deploy   # Specific user directory
```

### Glob Patterns

```yaml
file_transfer:
  allowed_paths:
    - /data/**           # Anything under /data
    - /home/*/uploads    # uploads dir for any user
    - /var/log/*.log     # Only .log files in /var/log
```

### All Paths (Testing Only)

```yaml
file_transfer:
  allowed_paths:
    - "*"  # Allow everything - DANGEROUS
```

:::danger Never Use in Production
The `["*"]` wildcard allows access to any path. Only use for testing in isolated environments.
:::

### Pattern Syntax

| Pattern | Matches | Does NOT Match |
|---------|---------|----------------|
| `/tmp` | `/tmp`, `/tmp/file.txt`, `/tmp/dir/file` | `/tmpdir` |
| `/tmp/*` | `/tmp/file.txt` | `/tmp/dir/file` |
| `/tmp/**` | `/tmp/file.txt`, `/tmp/dir/file`, `/tmp/a/b/c` | - |
| `/home/*/uploads` | `/home/alice/uploads`, `/home/bob/uploads` | `/home/uploads` |
| `*.log` | `app.log`, `/var/log/sys.log` | `app.txt` |

## File Size Limits

Control maximum transfer size:

```yaml
file_transfer:
  max_file_size: 104857600  # 100 MB
```

| Value | Size |
|-------|------|
| `0` | Unlimited |
| `10485760` | 10 MB |
| `104857600` | 100 MB |
| `524288000` | 500 MB (default) |
| `1073741824` | 1 GB |

:::tip Directory Transfers
For directory transfers, the limit applies to the compressed tar archive, not individual files.
:::

## Transfer Features

### Streaming

Files are streamed in chunks (16 KB) - no memory limits regardless of file size.

### Compression

Directories are automatically compressed with gzip during transfer.

### Permissions

File permissions (mode) are preserved during transfer.

### Resume

The CLI supports resuming interrupted transfers with `--resume`.

## Security Best Practices

1. **Restrict paths**: Only allow directories actually needed
2. **Set size limits**: Prevent disk exhaustion
3. **Use strong passwords**: 12+ character passwords
4. **Separate from shell**: Consider using different passwords
5. **Monitor usage**: Check logs for file operations

### Recommended Configurations by Use Case

**Deployment staging:**
```yaml
file_transfer:
  allowed_paths:
    - /opt/deploy/staging
  max_file_size: 104857600  # 100 MB
```

**Log collection:**
```yaml
file_transfer:
  allowed_paths:
    - /var/log/*.log
    - /var/log/**/*.log
  max_file_size: 52428800   # 50 MB
```

**General file sharing:**
```yaml
file_transfer:
  allowed_paths:
    - /tmp
    - /home/shared
  max_file_size: 524288000  # 500 MB
```

## Examples

### Minimal Access

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$..."
  max_file_size: 10485760   # 10 MB
  allowed_paths:
    - /tmp
```

### Deployment Agent

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$..."
  max_file_size: 524288000  # 500 MB
  allowed_paths:
    - /opt/app
    - /etc/app
```

### Log Collection Agent

```yaml
file_transfer:
  enabled: true
  password_hash: "$2a$10$..."
  max_file_size: 104857600  # 100 MB
  allowed_paths:
    - /var/log
```

## Environment Variables

```yaml
file_transfer:
  enabled: ${FILE_TRANSFER_ENABLED:-false}
  password_hash: "${FILE_TRANSFER_PASSWORD_HASH}"
  max_file_size: ${FILE_TRANSFER_MAX_SIZE:-524288000}
```

## Related

- [File Transfer Usage](/features/file-transfer) - How to use file transfer
- [Remote Shell](/configuration/shell) - Related remote access feature
- [Security Overview](/security/overview) - Security considerations
