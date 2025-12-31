---
title: hash
sidebar_position: 8
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-inspecting.png" alt="Mole generating hash" style={{maxWidth: '180px'}} />
</div>

# muti-metroo hash

Generate bcrypt password hashes for use in configuration files.

## Synopsis

```bash
muti-metroo hash [password] [flags]
```

## Description

The `hash` command generates bcrypt password hashes that can be used in Muti Metroo configuration files for authentication. Bcrypt is a secure, one-way hashing algorithm specifically designed for passwords.

The generated hash can be used in:

| Config Field | Purpose |
|--------------|---------|
| `socks5.auth.users[].password_hash` | SOCKS5 proxy authentication |
| `rpc.password_hash` | RPC command authorization |
| `file_transfer.password_hash` | File transfer authorization |

## Usage

### Interactive Mode (Recommended)

For security, use interactive mode which hides your password input:

```bash
muti-metroo hash
```

You will be prompted to enter and confirm your password:

```
Enter password:
Confirm password:
$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy
```

### Command Line Argument

You can also provide the password as an argument, but this is less secure as the password may be visible in shell history:

```bash
muti-metroo hash "mysecretpassword"
```

Output:

```
$2a$10$eXaMpLeHaShThAtIsUnIqUeAnDlOnGeNoUgHtObE
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cost` | 10 | bcrypt cost factor (4-31) |
| `-h, --help` | | Show help |

## Cost Factor

The cost factor determines how computationally expensive the hash is to generate and verify. Higher cost = more secure but slower.

| Cost | Time (approx) | Recommendation |
|------|---------------|----------------|
| 10 | ~100ms | Development/testing |
| 12 | ~400ms | Production (recommended) |
| 14 | ~1.5s | High security environments |

For production, use cost 12:

```bash
muti-metroo hash --cost 12
```

## Examples

### Generate Hash for SOCKS5 User

```bash
# Generate hash
muti-metroo hash --cost 12
Enter password:
Confirm password:
$2a$12$xYzAbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGhI

# Use in config.yaml
# socks5:
#   auth:
#     enabled: true
#     users:
#       - username: "admin"
#         password_hash: "$2a$12$xYzAbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGhI"
```

### Generate Hash for RPC Authentication

```bash
muti-metroo hash --cost 12

# Use in config.yaml
# rpc:
#   enabled: true
#   password_hash: "$2a$12$..."
#   whitelist:
#     - whoami
#     - hostname
```

### Generate Hash for File Transfer

```bash
muti-metroo hash --cost 12

# Use in config.yaml
# file_transfer:
#   enabled: true
#   password_hash: "$2a$12$..."
#   allowed_paths:
#     - /tmp
```

### Using Environment Variables

You can store the hash in an environment variable and reference it in config:

```bash
# Generate and export
export SOCKS5_PASSWORD_HASH=$(muti-metroo hash "mypassword")

# In config.yaml
# socks5:
#   auth:
#     users:
#       - username: "admin"
#         password_hash: "${SOCKS5_PASSWORD_HASH}"
```

### Scripting

For automation, you can pipe the password:

```bash
# Using echo (less secure - visible in process list)
echo -n "mypassword" | muti-metroo hash

# Using a file (more secure)
muti-metroo hash "$(cat /path/to/password/file)"
```

## Hash Format

The generated hash follows the bcrypt format:

```
$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy
|  |  |                                                      |
|  |  +-- 22-character salt + 31-character hash              |
|  +-- cost factor (10 = 2^10 iterations)                    |
+-- bcrypt algorithm identifier                               |
```

- `$2a$` - bcrypt algorithm version
- `10$` - cost factor (10 means 2^10 = 1024 iterations)
- Remaining characters - salt (22 chars) + hash (31 chars)

## Security Considerations

1. **Use interactive mode**: Avoid putting passwords in command line arguments when possible, as they may be logged in shell history.

2. **Use appropriate cost**: Balance security with performance. Cost 12 is recommended for production.

3. **Unique passwords**: Use different passwords for different components (SOCKS5, RPC, file transfer).

4. **Store hashes safely**: Even though hashes are one-way, treat configuration files with hashes as sensitive.

5. **Rotate passwords**: Periodically generate new hashes and update configurations.

## Alternative Methods

If you cannot use the Muti Metroo CLI, you can generate bcrypt hashes using:

### htpasswd (Apache)

```bash
htpasswd -bnBC 10 "" yourpassword | tr -d ':\n'
```

### Python

```python
import bcrypt
print(bcrypt.hashpw(b"yourpassword", bcrypt.gensalt(10)).decode())
```

### Node.js

```javascript
const bcrypt = require('bcrypt');
console.log(bcrypt.hashSync('yourpassword', 10));
```

### Go

```go
import "golang.org/x/crypto/bcrypt"
hash, _ := bcrypt.GenerateFromPassword([]byte("yourpassword"), 10)
fmt.Println(string(hash))
```

## Related

- [Authentication](/security/authentication) - Complete authentication guide
- [SOCKS5 Configuration](/configuration/socks5) - SOCKS5 proxy settings
- [RPC](/features/rpc) - Remote procedure calls
- [File Transfer](/features/file-transfer) - File transfer feature
