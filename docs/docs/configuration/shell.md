---
title: Remote Shell Configuration
sidebar_position: 12
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole configuring shell" style={{maxWidth: '180px'}} />
</div>

# Remote Shell Configuration

Execute commands on remote agents through the mesh. Remote shell supports both interactive mode (PTY for vim, htop) and streaming mode for simple commands and continuous output.

:::warning Security Feature
Remote shell is disabled by default. Enable only on agents that need remote administration, and always use password authentication with a strict command whitelist.
:::

**Minimal secure setup:**
```yaml
shell:
  enabled: true
  password_hash: "$2a$10$..."  # Generate with: muti-metroo hash
  whitelist:
    - whoami
    - hostname
```

## Configuration

```yaml
shell:
  enabled: false         # Disabled by default
  password_hash: ""      # bcrypt hash of shell password (required when enabled)
  whitelist: []          # Commands allowed (empty = none)
  timeout: 0s            # Command timeout (0 = no timeout)
  max_sessions: 0        # Max concurrent sessions (0 = unlimited)
```

## Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable remote shell access |
| `password_hash` | string | `""` | bcrypt hash of authentication password |
| `whitelist` | list | `[]` | Allowed command names |
| `timeout` | duration | `0s` | Maximum command execution time |
| `max_sessions` | int | `0` | Maximum concurrent shell sessions |

## Password Authentication

Shell access requires password authentication. Generate a password hash:

```bash
# Interactive (recommended - password not in history)
muti-metroo hash

# From argument
muti-metroo hash "your-secure-password"

# Custom cost factor (default: 10, higher = slower but more secure)
muti-metroo hash --cost 12
```

Use the generated hash in config:

```yaml
shell:
  enabled: true
  password_hash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
```

:::tip Password Requirements
Use a strong password (12+ characters). The bcrypt hash is stored in config, not the plaintext password.
:::

## Command Whitelist

The whitelist controls which commands can be executed:

### No Commands (Default)

```yaml
shell:
  whitelist: []  # No commands allowed
```

### Specific Commands

```yaml
shell:
  whitelist:
    - whoami
    - hostname
    - date
    - uptime
    - journalctl
```

### All Commands (Testing Only)

```yaml
shell:
  whitelist:
    - "*"  # Allow everything - DANGEROUS
```

:::danger Never Use in Production
The `["*"]` wildcard allows arbitrary command execution. Only use for testing in isolated environments.
:::

### Whitelist Rules

- Commands must be **base names only** (no paths)
- `bash` allows `bash`, not `/bin/bash`
- Arguments are not restricted - `journalctl -u muti-metroo -f` works if `journalctl` is whitelisted
- Shell built-ins work through the shell (e.g., `bash -c "echo hello"`)

## Session Limits

Control resource usage:

```yaml
shell:
  max_sessions: 10   # Max 10 concurrent shell sessions
  timeout: 5m        # Commands timeout after 5 minutes
```

| Setting | Value | Effect |
|---------|-------|--------|
| `max_sessions: 0` | Unlimited | No limit on concurrent sessions |
| `max_sessions: 10` | Limited | New sessions rejected when limit reached |
| `timeout: 0s` | No timeout | Commands run indefinitely |
| `timeout: 5m` | 5 minutes | Commands killed after timeout |

## Shell Modes

### Streaming Mode (Default)

For simple commands and continuous output:

```bash
muti-metroo shell <agent-id> whoami
muti-metroo shell <agent-id> journalctl -u muti-metroo -f
```

### Interactive Mode (PTY)

For programs requiring a terminal:

```bash
muti-metroo shell --tty <agent-id> htop
muti-metroo shell --tty <agent-id> vim /etc/config.yaml
```

## Platform Support

| Platform | Interactive (PTY) | Streaming |
|----------|-------------------|-----------|
| Linux | Yes | Yes |
| macOS | Yes | Yes |
| Windows | Yes (ConPTY) | Yes |

## Security Best Practices

1. **Use specific whitelist**: Only allow commands actually needed
2. **Set session limits**: Prevent resource exhaustion
3. **Use timeouts**: Prevent hung commands
4. **Strong passwords**: Use 12+ character passwords
5. **Audit usage**: Monitor shell access in logs

### Recommended Whitelists by Use Case

**Monitoring only:**
```yaml
whitelist:
  - whoami
  - hostname
  - uptime
  - date
  - df
  - free
```

**Log access:**
```yaml
whitelist:
  - journalctl
  - tail
  - cat
  - grep
```

**Full administration:**
```yaml
whitelist:
  - bash
  - sh
  - vim
  - nano
  - systemctl
  - journalctl
```

## Examples

### Monitoring Agent

```yaml
shell:
  enabled: true
  password_hash: "$2a$10$..."
  whitelist:
    - whoami
    - hostname
    - uptime
  max_sessions: 5
  timeout: 1m
```

### Administration Agent

```yaml
shell:
  enabled: true
  password_hash: "$2a$10$..."
  whitelist:
    - bash
    - vim
    - systemctl
    - journalctl
  max_sessions: 3
  timeout: 30m
```

### Development Agent

```yaml
shell:
  enabled: true
  password_hash: "$2a$10$..."
  whitelist:
    - "*"  # Testing only!
  max_sessions: 0
  timeout: 0s
```

## Environment Variables

```yaml
shell:
  enabled: ${SHELL_ENABLED:-false}
  password_hash: "${SHELL_PASSWORD_HASH}"
  timeout: "${SHELL_TIMEOUT:-5m}"
```

## Related

- [Remote Shell Usage](/features/shell) - How to use remote shell
- [Security Overview](/security/overview) - Security considerations
- [File Transfer](/configuration/file-transfer) - Related remote access feature
