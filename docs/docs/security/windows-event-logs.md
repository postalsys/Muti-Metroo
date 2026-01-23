---
title: Windows Event Logs
sidebar_position: 7
---

# Windows Event Logs

This page documents what Windows logs when Muti Metroo operates, based on controlled testing with Sysmon 15.15 on Windows 11.

## Event Sources

Windows provides multiple logging sources that can capture agent activity:

| Source | What It Captures | Default State |
|--------|------------------|---------------|
| Security Log | Process creation, network connections | Requires audit policy |
| System Log | Service installation, driver loading | Enabled |
| PowerShell Log | Script execution via shell | Requires policy |
| Sysmon | Process, network, file, DNS telemetry | Requires installation |

## Visibility by Role

Different agent roles generate different event patterns:

| Role | Logging Visibility | Reason |
|------|-------------------|--------|
| Transit | Very Low | Only relays encrypted data in memory |
| Exit | Low-Medium | Opens real TCP connections, may resolve DNS |
| Shell | Very High | Each command spawns a child process |
| File Transfer | High | File creation events logged |

## Log Entry Examples

The following are actual Sysmon Event ID 1 (Process Create) entries captured during testing.

### Agent Startup

When the agent starts, Sysmon logs the full process creation:

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 07:44:39.899
ProcessId: 1292
Image: C:\muti-metroo\muti-metroo.exe
CommandLine: .\muti-metroo.exe run -c .\config.yaml
CurrentDirectory: C:\muti-metroo\
User: Syler\LocalAdmin
IntegrityLevel: High
Hashes: MD5=B561DCB8F03DDD9D6F7F246AAF0CC883
        SHA256=73316C426119CAC8AF1613980BCF4F85EC510D71E6F15AFDB43D8CA32530B85E
        IMPHASH=B196866F0BF37F1F128FA153413B744F
ParentProcessId: 25724
ParentImage: C:\Windows\System32\cmd.exe
ParentCommandLine: "c:\windows\system32\cmd.exe" /c "cd C:\muti-metroo && .\muti-metroo.exe run -c .\config.yaml"
ParentUser: Syler\LocalAdmin
```

**What is revealed:**
- Full path to the binary
- Complete command line arguments including config file path
- Working directory
- User account running the process
- File hashes (MD5, SHA256, IMPHASH) for identification
- How the agent was started (parent process chain)

### Shell Command: whoami

When `whoami` is executed via remote shell:

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 07:56:12.456
ProcessId: 16360
Image: C:\Windows\System32\whoami.exe
FileVersion: 10.0.26100.1882 (WinBuild.160101.0800)
Description: whoami - displays logged on user information
CommandLine: whoami
CurrentDirectory: C:\muti-metroo\
User: Syler\LocalAdmin
IntegrityLevel: High
Hashes: MD5=956692DADC5B2CEB46E9219F7A5BEFFA
        SHA256=23240EF9F8B0A9A324110B1C2331DE31DC1B0E08F5359CB707E51A939AF56CD3
ParentProcessId: 1292
ParentImage: C:\muti-metroo\muti-metroo.exe
ParentCommandLine: .\muti-metroo.exe run -c .\config.yaml
ParentUser: Syler\LocalAdmin
```

**What is revealed:**
- The exact command executed
- That muti-metroo.exe spawned this process (parent relationship)
- The user context
- Working directory matches agent location

### Shell Command: netstat -an

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 07:56:42.159
ProcessId: 42532
Image: C:\Windows\System32\NETSTAT.EXE
Description: TCP/IP Netstat Command
CommandLine: netstat -an
CurrentDirectory: C:\muti-metroo\
User: Syler\LocalAdmin
Hashes: MD5=3EF03622959AC1765819B9087191440C
        SHA256=09BCA819AB371A2D4F62CA5114C1335C38FD196AE7956E2FC4D154BBE55C3F4C
ParentProcessId: 1292
ParentImage: C:\muti-metroo\muti-metroo.exe
ParentCommandLine: .\muti-metroo.exe run -c .\config.yaml
```

### Shell Command: PowerShell

PowerShell commands show the full script in the command line:

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 07:56:55.452
ProcessId: 32308
Image: C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe
Description: Windows PowerShell
CommandLine: powershell -Command "Get-Process | Select-Object -First 10 Name,Id,CPU"
CurrentDirectory: C:\muti-metroo\
User: Syler\LocalAdmin
Hashes: MD5=A97E6573B97B44C96122BFA543A82EA1
        SHA256=0FF6F2C94BC7E2833A5F7E16DE1622E5DBA70396F31C7D5F56381870317E8C46
ParentProcessId: 1292
ParentImage: C:\muti-metroo\muti-metroo.exe
ParentCommandLine: .\muti-metroo.exe run -c .\config.yaml
```

**What is revealed:**
- Complete PowerShell command including the script content
- If Script Block Logging (Event 4104) is enabled, even more detail is captured

### Shell Command: wevtutil (Event Log Export)

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 07:57:30.043
ProcessId: 24376
Image: C:\Windows\System32\wevtutil.exe
Description: Windows Event Log Utility
CommandLine: wevtutil epl Security C:\muti-metroo\Security.evtx
CurrentDirectory: C:\muti-metroo\
User: Syler\LocalAdmin
ParentProcessId: 1292
ParentImage: C:\muti-metroo\muti-metroo.exe
ParentCommandLine: .\muti-metroo.exe run -c .\config.yaml
```

### File Transfer: Upload (Writing Files to Agent)

When a file is uploaded to the agent, Sysmon may capture a FileCreate event (Event ID 11) if configured to monitor the target directory:

```
Event ID: 11 (FileCreate)
UtcTime: 2026-01-23 07:57:05.123
Image: C:\muti-metroo\muti-metroo.exe
TargetFilename: C:\muti-metroo\uploaded-file.txt
CreationUtcTime: 2026-01-23 07:57:05.123
User: Syler\LocalAdmin
```

**What is revealed:**
- The destination filename and path
- That muti-metroo.exe created the file
- Timestamp of file creation

**Note:** Default Sysmon configurations may not capture all file creations. The visibility depends on Sysmon rules targeting the specific directory or process.

### File Transfer: Download (Reading Files from Agent)

When a file is downloaded from the agent (read operation), **no Sysmon events are generated** by default.

**Why downloads are not logged:**
- Sysmon Event ID 11 only captures file **creation**, not file reads
- File read operations require Windows Object Access Auditing (Security Log Event 4663) which is disabled by default
- The agent simply reads the file content and sends it over the encrypted network stream

**What is NOT revealed:**
- Which files were read/downloaded
- When the download occurred
- File content (encrypted in transit)

This makes file downloads significantly less visible than file uploads.

## DLL Execution via rundll32

When Muti Metroo runs as a DLL loaded by rundll32.exe, the log entries differ significantly from the standalone executable.

### DLL Agent Startup

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 08:27:06.568
ProcessId: 44208
Image: C:\Windows\System32\rundll32.exe
FileVersion: 10.0.26100.7309 (WinBuild.160101.0800)
Description: Windows host process (Rundll32)
CommandLine: "C:\Windows\system32\rundll32.exe" C:\muti-metroo\muti-metroo.dll,Run C:\muti-metroo\config.yaml
CurrentDirectory: C:\Users\LocalAdmin\
User: NT AUTHORITY\SYSTEM
IntegrityLevel: System
Hashes: MD5=BB65029E6ADC1C632492BCFB948E495B
        SHA256=4193F529BFF2729769D82CB83EE206D2AF16C5920...
        IMPHASH=C8B70D465C35D895C4171BAF042BB63A
ParentProcessId: 29176
ParentImage: C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe
ParentUser: Syler\LocalAdmin
```

### Shell Command via DLL

When a shell command is executed through the DLL-based agent:

```
Event ID: 1 (Process Create)
UtcTime: 2026-01-23 08:30:19.908
ProcessId: 17504
Image: C:\Windows\System32\wevtutil.exe
Description: Windows Event Log Utility
CommandLine: wevtutil qe Microsoft-Windows-Sysmon/Operational /c:20 /rd:true /f:text
CurrentDirectory: C:\Windows\system32\
User: NT AUTHORITY\SYSTEM
ParentProcessId: 43360
ParentImage: C:\Windows\System32\rundll32.exe
ParentCommandLine: "rundll32.exe" C:\muti-metroo\muti-metroo.dll,Run C:\muti-metroo\config.yaml
ParentUser: NT AUTHORITY\SYSTEM
```

### Key Differences: EXE vs DLL

| Aspect | Standalone EXE | DLL via rundll32 |
|--------|---------------|------------------|
| Image field | `muti-metroo.exe` | `rundll32.exe` (Windows binary) |
| Hashes logged | Custom binary hash | rundll32.exe hash (Windows) |
| DLL visibility | N/A | DLL path in CommandLine only |
| ParentImage for shell | `muti-metroo.exe` | `rundll32.exe` |
| Binary identification | Direct hash match | Must parse CommandLine for DLL path |

**Implications:**

1. **Hash-based identification fails** - The logged hash is for rundll32.exe (a Windows system binary), not the Muti Metroo DLL. The DLL itself is not hashed in process creation events.

2. **DLL path is visible** - The CommandLine field reveals the full DLL path (`C:\muti-metroo\muti-metroo.dll,Run`), but this requires parsing rather than direct field matching.

3. **Parent attribution changes** - Shell commands show `ParentImage: rundll32.exe` instead of `muti-metroo.exe`, which is a common Windows process.

4. **CurrentDirectory differs** - DLL execution may use `C:\Windows\system32\` as the working directory instead of the agent's installation path.

## Attribution Analysis

### Fully Attributable

The following can be directly traced to Muti Metroo:

| Information | How It's Revealed |
|-------------|-------------------|
| Agent binary path | `Image` field in process create event |
| Agent command line | `CommandLine` field shows config file path |
| Agent binary hash | `Hashes` field enables identification |
| Shell commands | `ParentImage` points to muti-metroo.exe |
| Command arguments | Full command line visible |
| User context | `User` field shows executing account |
| Working directory | `CurrentDirectory` reveals agent location |

### Parent-Child Attribution

Every shell command creates a clear attribution chain:

```
cmd.exe (or sshd.exe, schtasks.exe, etc.)
  -> muti-metroo.exe run -c config.yaml
      -> whoami.exe
      -> netstat.exe -an
      -> powershell.exe -Command "..."
```

The `ParentImage` and `ParentCommandLine` fields explicitly show that muti-metroo.exe spawned each command. This is the most reliable attribution indicator.

### Not Directly Attributable

| Activity | Visibility |
|----------|------------|
| Stream content | Encrypted, not logged |
| Transit traffic | In-memory only, no process events |
| Remote peer identity | Not in Windows logs (only in agent logs) |
| SOCKS5 client requests | Not logged unless exit role |
| Shell command source | Cannot determine which peer sent the command |

**Command source attribution:** Windows event logs show *that* a command was executed and *what* was executed, but cannot identify *who* sent the command or *where* it originated from. The remote peer that initiated the shell session is only visible in the agent's own debug logs, not in Windows event logs. Even Sysmon network connection events only show mesh peer connections, not which specific connection triggered a particular command.

### Network Connection Attribution

QUIC (UDP) connections may not appear in default Sysmon configurations. When network events are captured (Sysmon Event ID 3), they show:

- Source/destination IP and port
- Process that initiated the connection
- Protocol (TCP/UDP)

However, the encrypted nature of mesh traffic means only connection metadata is visible, not content.

## Forensic Visibility Summary

| Activity | Process Events | File Events | Network Events | Content Visible |
|----------|---------------|-------------|----------------|-----------------|
| Agent startup (EXE) | Yes | Binary on disk | Listener ports | N/A |
| Agent startup (DLL) | Yes (rundll32) | DLL on disk | Listener ports | DLL path in cmdline |
| Transit relay | No | No | Connection metadata | No |
| Exit connections | No | No | TCP connections | No |
| Shell commands (EXE) | Yes (parent: muti-metroo.exe) | No | No | Command line only |
| Shell commands (DLL) | Yes (parent: rundll32.exe) | No | No | Command line only |
| File upload | No | Maybe (depends on Sysmon config) | No | Filename only |
| File download | No | No | No | No |
| Service install | Yes | Yes (registry) | No | N/A |

## Key Observations

1. **Shell commands are highly visible** - Each command spawns a child process with muti-metroo.exe clearly shown as the parent. The full command line is logged.

2. **Transit generates minimal logs** - When acting purely as a relay, only the agent startup is logged. Stream forwarding happens in memory.

3. **Binary hashes are logged** - The SHA256, MD5, and IMPHASH are logged at startup, allowing identification regardless of filename.

4. **Working directory reveals location** - All shell commands show `CurrentDirectory: C:\muti-metroo\` which indicates the agent's installation path.

5. **User context is preserved** - The Windows user running the agent is logged, providing attribution to a specific account.

## Related Topics

- [Traffic Patterns & Detection](/security/traffic-patterns) - Network-level visibility
- [Shell](/features/shell) - Remote shell feature
- [File Transfer](/features/file-transfer) - File transfer feature
