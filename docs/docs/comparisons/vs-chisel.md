---
title: Chisel vs Muti Metroo
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-thinking.png" alt="Mole comparing options" style={{maxWidth: '180px'}} />
</div>

# Chisel vs Muti Metroo

Both Chisel and Muti Metroo create encrypted tunnels for TCP and UDP traffic, but they take different architectural approaches. This comparison helps you choose the right tool for your use case.

## Quick Comparison

| Aspect | Chisel | Muti Metroo + Mutiauk |
|--------|--------|----------------------|
| **Architecture** | Client-Server | Mesh network (any topology) |
| **Transport** | HTTP/WebSocket | QUIC, HTTP/2, WebSocket |
| **Encryption** | SSH (crypto/ssh) | TLS 1.3 + E2E (ChaCha20-Poly1305) |
| **Multi-hop** | Manual chaining | Native flood-based routing |
| **SOCKS5 Proxy** | Yes | Yes |
| **UDP Support** | Yes (tunneled) | Yes (SOCKS5 UDP ASSOCIATE) |
| **Reverse Tunneling** | Yes (R: prefix) | Yes (port forwarding) |
| **Authentication** | Username/password file | TLS certificates + mTLS |
| **TUN Interface** | No | Yes (Mutiauk) |
| **Auto-reconnect** | Yes | Yes |
| **ICMP Support** | No | Yes |
| **File Transfer** | No | Yes (built-in) |
| **Remote Shell** | No | Yes (authenticated) |
| **Platform Support** | Linux, macOS, Windows | Linux, macOS, Windows |

## Architecture Comparison

### Chisel: Client-Server Model

Chisel uses a traditional **client-server model** where clients connect to a central server:

```mermaid
flowchart TB
    subgraph Your Machine
        Client[Chisel Client]
    end

    subgraph Server Host
        Server[Chisel Server]
    end

    subgraph Target Network
        Service1[Service A]
        Service2[Service B]
    end

    Client -->|"HTTP/WebSocket + SSH"| Server
    Server --> Service1
    Server --> Service2
```

**How it works:**
1. Start Chisel server on a host with access to target services
2. Connect Chisel client specifying which ports to forward
3. Traffic flows: Client -> Server -> Target service
4. Single connection multiplexes all tunnels

### Muti Metroo: Mesh Network

Muti Metroo creates a **mesh network** where you configure peer connections between agents and routes propagate automatically:

```mermaid
flowchart TB
    subgraph Your Machine
        App[Application]
        TUN[Mutiauk TUN]
    end

    subgraph Mesh Network
        A[Agent A<br/>Ingress]
        B[Agent B<br/>Transit]
        C[Agent C<br/>Exit]
    end

    subgraph Target Network
        Service[Target Service]
    end

    App --> TUN
    TUN -->|"SOCKS5"| A
    A <-->|"QUIC/HTTP/2/WS"| B
    B <-->|"QUIC/HTTP/2/WS"| C
    C --> Service
```

**How it works:**
1. Deploy agents on available hosts
2. Configure peer connections between agents (any topology)
3. Exit agents advertise routes via flood-based routing
4. Traffic finds optimal path through the mesh

## Tunneling Modes

### Chisel Tunneling

Chisel supports three tunneling modes with a concise syntax:

**Local Port Forward:**
```bash
# Forward local port 3000 to remote service
chisel client server:8080 3000:internal-host:80

# Access via localhost
curl http://localhost:3000
```

**Reverse Port Forward:**
```bash
# Expose local service through server
chisel client server:8080 R:8080:localhost:3000

# Service now accessible at server:8080
```

**SOCKS5 Proxy:**
```bash
# Server with SOCKS5 enabled
chisel server --socks5

# Client connects
chisel client server:8080 socks

# Use SOCKS proxy
curl --socks5 localhost:1080 http://internal-host/
```

### Muti Metroo Tunneling

Muti Metroo uses SOCKS5 as the primary ingress with declarative exit routing:

**SOCKS5 Proxy (default):**
```yaml
# Ingress agent config
socks5:
  enabled: true
  address: "127.0.0.1:1080"
```

```bash
# Access any destination via SOCKS5
curl --socks5 localhost:1080 http://internal-host/
```

**Transparent TUN (with Mutiauk):**
```bash
# Start Mutiauk for transparent routing
sudo mutiauk daemon start

# All traffic to configured routes flows through mesh
curl http://internal-host/
ping internal-host
```

**Port Forwarding (reverse tunnel):**
```yaml
# Endpoint agent - where service runs
forward:
  endpoints:
    - key: "my-service"
      target: "localhost:3000"

# Listener agent - where to expose
forward:
  listeners:
    - key: "my-service"
      address: ":8080"
```

## Encryption Comparison

### Chisel: SSH Encryption

Chisel uses Go's `crypto/ssh` library for encryption:

- ECDSA key pairs generated at startup
- SSH protocol encryption
- Optional fingerprint verification
- Persistent keys via `--keyfile`

```bash
# Server displays fingerprint at startup
chisel server --port 8080
# Fingerprint: ab:cd:ef:...

# Client validates fingerprint
chisel client --fingerprint ab:cd:ef:... server:8080 3000
```

### Muti Metroo: TLS + End-to-End

Muti Metroo provides two layers of encryption:

**Transport Layer (TLS 1.3):**
- Encrypts peer-to-peer connections
- Certificate-based authentication
- Optional mTLS for mutual authentication

**End-to-End Layer:**
- X25519 key exchange per stream
- ChaCha20-Poly1305 encryption
- Transit agents cannot decrypt traffic

```mermaid
flowchart LR
    subgraph E2E["End-to-End Encrypted"]
        Client[Ingress]
        Exit[Exit Agent]
    end

    Client -->|"TLS + E2E"| Transit[Transit Agent]
    Transit -->|"TLS + E2E"| Exit

    style Transit fill:#f9f9f9,stroke:#ccc
```

The transit agent relays encrypted frames it cannot read - only ingress and exit can decrypt.

## Multi-hop Capabilities

### Chisel: Manual Chaining

To reach services through multiple hops, chain Chisel instances. Unlike SSH (which requires SSH servers on each hop), Chisel runs as a standalone binary on any platform - including Windows. However, multi-hop still requires manual configuration at each step.

**Double pivot scenario:**

```mermaid
flowchart LR
    subgraph Your Machine
        Client[Chisel Client]
    end

    subgraph DMZ["DMZ (Linux)"]
        Server1[Chisel Server]
        Client1[Chisel Client]
    end

    subgraph Internal["Internal (Windows)"]
        Server2[Chisel Server]
    end

    subgraph Target
        Final[Target Network]
    end

    Client -->|"8080"| Server1
    Client1 -->|"R:9090"| Server2
    Server2 --> Final
```

**Step-by-step setup:**

```bash
# Step 1: Start server on DMZ host (Linux)
chisel server --port 8080

# Step 2: On DMZ host, run client to create reverse tunnel to internal
chisel client dmz-server:8080 R:9090:internal-server:8080

# Step 3: Start server on internal host (Windows works fine)
chisel.exe server --port 8080

# Step 4: From your machine, connect through the chain
chisel client dmz-server:9090 3000:final-target:80
```

**Platform advantage over SSH**: Unlike SSH jump hosts, Chisel runs natively on Windows without requiring OpenSSH Server installation. You can deploy `chisel.exe` on Windows machines and they participate in the chain.

**Complexity remains**: Each hop still requires:
- Deploying Chisel binary to intermediate hosts
- Running both server AND client on transit hosts
- Manual reverse tunnel configuration for each hop
- Managing multiple processes across the chain

### Muti Metroo: Native Routing

Routes propagate automatically through flood-based routing - same binary, simpler setup:

```mermaid
flowchart LR
    subgraph Your Machine
        App[Application]
    end

    subgraph DMZ["DMZ (Linux)"]
        AgentA[Agent A]
    end

    subgraph Internal["Internal (Windows)"]
        AgentB[Agent B]
    end

    subgraph Target
        Final[10.0.0.0/8]
    end

    App -->|SOCKS5| AgentA
    AgentA <-->|HTTP/2| AgentB
    AgentB --> Final
```

```yaml
# Agent B config (Windows) - just advertise routes
exit:
  cidr_routes:
    - cidr: "10.0.0.0/8"
# Routes automatically propagate to Agent A
```

**Key differences:**
- Single agent process per host (not server + client)
- Routes propagate automatically through the mesh
- No manual reverse tunnel chaining
- Add or remove agents without reconfiguring others

```bash
# Access any destination - routing handled automatically
curl --socks5 localhost:1080 http://10.20.30.40/
```

## Platform Considerations

### Cross-Platform Support

Both Chisel and Muti Metroo are written in Go and run natively on Windows, Linux, and macOS without additional dependencies. This is a significant advantage over SSH-based solutions, where Windows machines typically lack SSH servers.

| Platform | Chisel | Muti Metroo |
|----------|--------|-------------|
| Linux | Yes | Yes |
| macOS | Yes | Yes |
| Windows | Yes | Yes |
| No dependencies | Yes | Yes |

### Mixed Environment Complexity

While both tools run on Windows, their multi-hop approaches differ significantly:

**Chisel in mixed environments:**

```mermaid
flowchart LR
    subgraph YourMachine["Your Machine"]
        Client[Chisel Client]
    end

    subgraph Linux["Linux Host"]
        Server1[Chisel Server]
        Client1[Chisel Client]
    end

    subgraph Windows["Windows Host"]
        Server2[Chisel Server]
        Client2[Chisel Client]
    end

    subgraph Target["Target"]
        Final[Target Network]
    end

    Client --> Server1
    Client1 --> Server2
    Client2 --> Final

    Linux -.- Note1["2 processes"]
    Windows -.- Note2["2 processes"]

    style Note1 fill:none,stroke:none
    style Note2 fill:none,stroke:none
```

- Each intermediate host runs TWO processes (server + client)
- Manual reverse tunnel configuration at each hop
- Must coordinate ports and addresses across the chain
- Adding a new hop requires reconfiguring existing connections

**Muti Metroo in mixed environments:**

```mermaid
flowchart LR
    subgraph YourMachine["Your Machine"]
        App[Application]
    end

    subgraph Linux["Linux Host"]
        AgentA[Agent]
    end

    subgraph Windows["Windows Host"]
        AgentB[Agent]
    end

    subgraph Target["Target"]
        Final[Target Network]
    end

    App -->|SOCKS5| AgentA
    AgentA <--> AgentB
    AgentB --> Final

    Linux -.- Note1["1 process"]
    Windows -.- Note2["1 process"]

    style Note1 fill:none,stroke:none
    style Note2 fill:none,stroke:none
```

- Each host runs ONE agent process
- Peer connections are bidirectional
- Routes propagate automatically
- Adding a new hop requires no changes to existing agents

### Operational Differences

| Aspect | Chisel | Muti Metroo |
|--------|--------|-------------|
| Processes per hop | 2 (server + client) | 1 (agent) |
| Configuration | CLI flags per instance | Single YAML config |
| Route management | Manual reverse tunnels | Automatic propagation |
| Adding new hop | Reconfigure chain | Just connect new agent |
| Service mode | Manual (nohup, screen) | Built-in (systemd, launchd, Windows Service) |

## Reverse Tunneling Comparison

### Chisel Reverse Tunnels

Use the `R:` prefix to expose local services through the server:

```bash
# Expose local web server
chisel client server:8080 R:8080:localhost:3000

# Expose local SSH
chisel client server:8080 R:2222:localhost:22

# Multiple reverse tunnels
chisel client server:8080 R:8080:localhost:3000 R:2222:localhost:22
```

The server must be reachable from your client.

### Muti Metroo Port Forwarding

Configure endpoints (where services run) and listeners (where to expose):

```yaml
# Agent on machine with the service
forward:
  endpoints:
    - key: "web-server"
      target: "localhost:3000"
    - key: "ssh-access"
      target: "localhost:22"
```

```yaml
# Agent where you want to access the service
forward:
  listeners:
    - key: "web-server"
      address: ":8080"
    - key: "ssh-access"
      address: ":2222"
```

Services are discovered via routing - works across multi-hop paths.

## Authentication

### Chisel Authentication

Optional username/password authentication via JSON file:

```json
// users.json
{
  "user1": "password1",
  "user2": "password2"
}
```

```bash
# Server with auth
chisel server --authfile users.json

# Client with credentials
chisel client --auth user1:password1 server:8080 3000
```

### Muti Metroo Authentication

TLS certificate-based authentication:

```yaml
tls:
  ca: "./certs/ca.crt"
  cert: "./certs/agent.crt"
  key: "./certs/agent.key"
  require_client_cert: true  # mTLS
```

Additional authentication for features:
- SOCKS5: Optional username/password
- Shell: bcrypt password hash + command whitelist
- File transfer: bcrypt password hash + path whitelist

## Feature Comparison

| Feature | Chisel | Muti Metroo |
|---------|--------|-------------|
| Local port forward | Yes | Via SOCKS5/Mutiauk |
| Remote port forward | Yes (R: prefix) | Yes (port forwarding) |
| SOCKS5 proxy | Yes | Yes |
| UDP tunneling | Yes | Yes |
| ICMP (ping) | No | Yes |
| TUN interface | No | Yes (Mutiauk) |
| File transfer | No | Yes |
| Remote shell | No | Yes |
| Web dashboard | No | Yes |
| Let's Encrypt | Yes | No (bring your own certs) |
| Config file | No (CLI only) | Yes (YAML) |
| Service mode | No | Yes (systemd, launchd, Windows) |
| HTTP API | No | Yes |

## When to Use Chisel

Chisel excels when:

- **Simple point-to-point tunneling**: Need to forward a few ports quickly
- **Single server access**: One server provides access to target network (no multi-hop needed)
- **Quick deployment**: No config files needed, just CLI flags
- **SSH-style encryption**: Prefer SSH protocol encryption
- **Let's Encrypt**: Want automatic TLS certificates
- **Cross-platform single-hop**: Need to tunnel through a Windows machine (unlike SSH)

**Typical workflow:**
```bash
# Start server (target network)
chisel server --port 8080 --socks5

# Connect client (your machine)
chisel client https://server.example.com:8080 socks

# Use SOCKS proxy
curl --socks5 localhost:1080 http://internal-service/
```

## When to Use Muti Metroo

Muti Metroo excels when:

- **Complex topologies**: Multi-site, chain, tree, or mesh networks
- **Native multi-hop**: Traffic needs to traverse multiple network boundaries without manual chaining
- **Mixed OS environments**: Simpler multi-hop setup across Windows/Linux/macOS (single agent per host)
- **E2E encryption**: Transit nodes must not see your data
- **UDP/ICMP support**: Need to tunnel more than TCP
- **Transparent routing**: Want TUN-based access without SOCKS configuration
- **Additional features**: File transfer, remote shell, web dashboard
- **Always-on infrastructure**: Persistent mesh with automatic reconnection and service mode

**Typical workflow:**
```bash
# Deploy agents (any topology)
muti-metroo run -c config.yaml

# Connect via SOCKS5
curl --socks5 localhost:1080 http://internal/

# Or transparent with Mutiauk
sudo mutiauk daemon start
curl http://internal/
ping internal-server
```

## Migration from Chisel

If you're considering switching from Chisel:

| Chisel Concept | Muti Metroo Equivalent |
|----------------|------------------------|
| Server | Exit agent (advertises routes) |
| Client | Ingress agent (provides SOCKS5) |
| `local:remote` | SOCKS5 proxy (any destination) |
| `R:server:local` | Port forwarding (endpoint + listener) |
| `socks` | SOCKS5 enabled by default |
| `--authfile` | TLS certificates + mTLS |

### Key Differences

1. **No explicit port mapping**: Use SOCKS5 for any destination
2. **Declarative config**: YAML files instead of CLI flags
3. **Automatic routing**: Exit agents advertise what they can reach
4. **TUN option**: Mutiauk for transparent routing

## Summary

| Choose | When You Need |
|--------|---------------|
| **Chisel** | Simple port forwarding, quick setup, single-hop tunneling, CLI-only workflow |
| **Muti Metroo** | Complex topologies, native multi-hop without manual chaining, E2E encryption, UDP/ICMP, TUN interface, always-on mesh |

Both tools run natively on Windows, Linux, and macOS - unlike SSH which requires server installation on Windows. However, Chisel's multi-hop approach requires manual server+client chaining at each hop, while Muti Metroo handles routing automatically with a single agent per host.

Chisel is excellent for quick, simple tunneling with minimal setup. Muti Metroo provides a more comprehensive solution for complex networking scenarios with native multi-hop routing, end-to-end encryption, and transparent TUN-based routing.

## See Also

- [Core Concepts - Architecture](/concepts/architecture) - How Muti Metroo works
- [Mutiauk TUN Interface](/mutiauk) - Transparent routing with TUN
- [Features - Port Forwarding](/features/port-forwarding) - Reverse tunnel equivalent
- [Security - E2E Encryption](/security/e2e-encryption) - How E2E encryption works
