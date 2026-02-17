# Remote Shell

Execute shell commands on remote agents with support for both streaming and interactive modes:

- **Streaming mode**: Standard command execution (default)
- **Interactive mode**: Full terminal support for vim, bash, htop

## Configuration

```yaml
shell:
  enabled: false              # Disabled by default (security)
  whitelist: []               # Commands allowed (empty = none)
  password_hash: ""           # bcrypt hash of shell password
  timeout: 0s                 # Command timeout (0 = no timeout)
  max_sessions: 0             # Concurrent sessions (0 = unlimited)
```

**Warning**: Shell access is a powerful feature. Enable only when needed and configure security controls carefully.

## Security Model

### Command Whitelist

Only whitelisted commands can run:

| Configuration | Behavior |
|--------------|----------|
| `[]` (empty) | No commands allowed (default) |
| `["*"]` | All commands allowed (testing only!) |
| `["bash", "vim", "whoami"]` | Only specified commands |

```yaml
shell:
  whitelist:
    - bash
    - sh
    - whoami
    - hostname
    - cat
    - ls
```

### Password Authentication

Generate a bcrypt password hash:

```bash
muti-metroo hash --cost 12
Enter password:
Confirm password:
$2a$12$...
```

Configure in the agent:

```yaml
shell:
  enabled: true
  whitelist:
    - bash
  password_hash: "$2a$12$..."
```

## Modes

### Streaming Mode (Default)

Standard execution without PTY allocation:

- Separate stdout and stderr streams
- Commands run until exit and return an exit code
- No terminal control characters
- Good for simple commands and log following

```bash
# Simple commands
muti-metroo shell abc123 whoami
muti-metroo shell abc123 ls -la /tmp

# Long-running streaming commands
muti-metroo shell abc123 journalctl -u muti-metroo -f
muti-metroo shell abc123 tail -f /var/log/syslog
```

### Interactive Mode (--tty)

Allocates a PTY (pseudo-terminal) on the remote agent:

- Full terminal emulation
- Supports terminal resize
- Works with interactive programs (vim, less, htop)
- Single combined stdout/stderr stream

```bash
# Interactive bash
muti-metroo shell --tty abc123 bash

# Edit files with vim
muti-metroo shell --tty abc123 vim /etc/config.yaml

# System monitoring
muti-metroo shell --tty abc123 htop
```

## CLI Usage

```bash
muti-metroo shell [flags] <agent-id> [command] [args...]
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--agent` | `-a` | `localhost:8080` | Agent HTTP API address |
| `--password` | `-p` | | Shell password |
| `--timeout` | `-t` | `0` | Session timeout (seconds) |
| `--tty` | | | Interactive mode with PTY |

### Examples

```bash
# Simple command (streaming mode)
muti-metroo shell abc123 whoami

# Follow logs (streaming mode)
muti-metroo shell abc123 journalctl -f

# Interactive bash
muti-metroo shell --tty abc123 bash

# With password
muti-metroo shell -p secret abc123 whoami

# Via different agent's HTTP API
muti-metroo shell -a 192.168.1.10:8080 --tty abc123 top

# With timeout (auto-terminate after 300 seconds)
muti-metroo shell -t 300 abc123 bash
```

## Platform Support

| Platform | Interactive (PTY) | Streaming |
|----------|:-----------------:|:---------:|
| Linux | Yes | Yes |
| macOS | Yes | Yes |
| Windows | Yes (ConPTY) | Yes |

Windows uses ConPTY (Windows Pseudo Console) for interactive sessions, available on Windows 10 version 1809 and later.

## Shell Detection

Each agent automatically detects installed shells at startup and advertises them to the mesh. This lets you see which shells are available on any agent through the `/api/nodes` endpoint.

Probed shells by platform:

- **Linux/macOS**: bash, sh, zsh, fish, ash, dash, ksh
- **Windows**: powershell.exe, pwsh.exe, cmd.exe

Shell detection is separate from the command whitelist. A detected shell only reports what is installed -- the whitelist controls what is allowed to execute.

### Windows PowerShell

```bash
# Interactive PowerShell on Windows target
muti-metroo shell --tty abc123 powershell

# Or cmd.exe
muti-metroo shell --tty abc123 cmd
```

## Example Configuration

### Minimal (Testing)

```yaml
shell:
  enabled: true
  whitelist:
    - "*"           # Allow all commands (TESTING ONLY)
```

### Production

```yaml
shell:
  enabled: true
  whitelist:
    - bash
    - sh
    - whoami
    - hostname
    - cat
    - ls
    - ps
    - netstat
  password_hash: "$2a$12$..."
  timeout: 3600s    # 1 hour max session
  max_sessions: 5   # Limit concurrent sessions
```

## Troubleshooting

### Command Rejected

```
Error: command not in whitelist
```

Add the command to the whitelist:

```yaml
shell:
  whitelist:
    - your-command
```

### Authentication Failed

```
Error: shell authentication failed
```

Verify the password matches the configured hash. Generate a new hash if needed:

```bash
muti-metroo hash
```

### PTY Not Working

Ensure you're using the `--tty` flag for interactive programs:

```bash
# Wrong (no terminal)
muti-metroo shell abc123 vim

# Correct
muti-metroo shell --tty abc123 vim
```
