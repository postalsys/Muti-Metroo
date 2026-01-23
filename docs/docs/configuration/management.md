---
title: Management Key
sidebar_position: 15
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-thinking.png" alt="Mole configuring management keys" style={{maxWidth: '180px'}} />
</div>

# Management Key Configuration

Encrypt mesh topology data so only designated management nodes can view the network structure. Field agents see only encrypted blobs - they cannot discover other agents or routes in the mesh.

:::info OPSEC Feature
Management key encryption is an optional security feature for sensitive environments. Most deployments don't need it.
:::

## When to Use

Use management keys when:

- Field agents should not know the mesh topology
- Captured agents should not reveal network structure
- Multi-tenant environments with compartmentalization needs
- High-security deployments requiring topology privacy

## How It Works

```
Field Agent          Management Node
     |                    |
     | NodeInfo (encrypted)
     |------------------>|
     |                    | (can decrypt)
     |                    |
     | Routes (encrypted) |
     |------------------>|
     |                    | (can decrypt)
```

1. **Field agents** have only the public key - they encrypt but cannot decrypt
2. **Management nodes** have the private key - they can decrypt and view topology
3. **Encrypted data**: Node info, route paths, topology advertisements

## Configuration

### Generate Keys

```bash
# Generate new keypair
muti-metroo management-key generate

# Output:
# Private Key: e5f6a7b8c9d012345678901234567890...
# Public Key:  a1b2c3d4e5f6789012345678901234567890...
```

### Field Agent (Encrypt Only)

Field agents have only the public key:

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
```

These agents:
- Encrypt their node info before advertising
- Cannot decrypt other agents' node info
- Cannot see the mesh topology in their dashboard

### Management Node (Can Decrypt)

Management nodes have both keys:

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"
```

These nodes:
- Encrypt their own node info (using public key)
- Decrypt other agents' node info (using private key)
- Can view full mesh topology in dashboard

## Options

### Topology Encryption Keys

| Option | Type | Description |
|--------|------|-------------|
| `public_key` | string | 64-character hex X25519 public key |
| `private_key` | string | 64-character hex X25519 private key |

### Command Signing Keys

| Option | Type | Description |
|--------|------|-------------|
| `signing_public_key` | string | 64-character hex Ed25519 public key (32 bytes) |
| `signing_private_key` | string | 128-character hex Ed25519 private key (64 bytes) |

## Key Distribution

### All Agents Need Same Public Key

For encryption to work across the mesh, all agents must use the **same public key**:

```yaml
# Agent A (field)
management:
  public_key: "a1b2c3d4..."

# Agent B (field)
management:
  public_key: "a1b2c3d4..."  # Same key

# Agent C (management)
management:
  public_key: "a1b2c3d4..."  # Same key
  private_key: "e5f6a7b8..."  # Only on management nodes
```

### Never Distribute Private Key

The private key should only exist on management nodes. Never:
- Include private key in field agent configs
- Store private key in shared repositories
- Transmit private key over untrusted channels

## What Gets Encrypted

| Data | Encrypted | Effect |
|------|-----------|--------|
| Node info (display name, roles) | Yes | Field agents see encrypted blobs |
| Route paths | Yes | Field agents can't trace routes |
| Agent IDs in advertisements | Yes | Topology hidden from field agents |
| Actual stream data | No | E2E encryption handles this separately |
| CIDR routes | No | Routing still works for field agents |

## Dashboard Behavior

### Without Management Key

Dashboard shows full topology - all agents, names, connections.

### Field Agent (Public Key Only)

Dashboard shows:
- Own agent info
- Connected peers (as encrypted IDs)
- Routes (destinations only, not paths)

Cannot see:
- Other agent names
- Full mesh topology
- Route paths through mesh

### Management Node (Both Keys)

Dashboard shows complete topology - same as without management keys.

## Deployment Patterns

### Centralized Management

```
        Management Server
        (has private key)
              |
    +---------+---------+
    |         |         |
  Field A   Field B   Field C
  (public)  (public)  (public)
```

### Distributed Management

```
  Management A          Management B
  (both keys)           (both keys)
       |                     |
   +---+---+             +---+---+
   |       |             |       |
 Field   Field         Field   Field
```

## Examples

### Field Agent

```yaml
agent:
  display_name: "Field-Alpha"

management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"

# No private_key - cannot decrypt topology
```

### Management Node

```yaml
agent:
  display_name: "Management-Central"

management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"

http:
  enabled: true
  dashboard: true  # Can view full topology
```

### No Encryption (Default)

```yaml
# No management section = no topology encryption
# All agents can see full mesh topology
```

## Command Signing Keys

Signing keys authenticate sleep/wake commands using Ed25519 signatures. This prevents unauthorized parties from putting your mesh to sleep.

:::info Separate from Topology Encryption
Signing keys (Ed25519) are independent from topology encryption keys (X25519). You can use one, both, or neither depending on your security requirements.
:::

### Generate Signing Keys

```bash
# Generate new Ed25519 keypair
muti-metroo signing-key generate

# Output:
# Signing Private Key: e5f6a7b8c9d012345678901234567890...
# Signing Public Key:  a1b2c3d4e5f6789012345678901234567890...
```

### All Agents (Verify Only)

All agents need the public key to verify incoming commands:

```yaml
management:
  signing_public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
```

These agents:
- Verify signatures on incoming sleep/wake commands
- Reject unsigned or incorrectly signed commands
- Cannot issue sleep/wake commands themselves

### Operator Nodes (Can Sign)

Operator nodes need both keys to issue signed commands:

```yaml
management:
  signing_public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  signing_private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890efe5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"
```

These nodes:
- Sign outgoing sleep/wake commands
- Can trigger mesh-wide sleep or wake
- Should be limited to authorized operators

### Combined Configuration

You can use both topology encryption and command signing:

```yaml
management:
  # Topology encryption (X25519)
  public_key: "a1b2c3d4..."
  private_key: "e5f6a7b8..."  # Only on management nodes

  # Command signing (Ed25519)
  signing_public_key: "1234abcd..."
  signing_private_key: "5678efgh..."  # Only on operators
```

### Backward Compatibility

:::warning
Agents without `signing_public_key` configured will accept ALL sleep/wake commands, signed or unsigned. For full protection, deploy the public key to every agent in your mesh.
:::

## Security Considerations

1. **Key compromise**: If private key is compromised, generate new keypair and redeploy
2. **Key rotation**: No built-in rotation - requires config update across all agents
3. **Mixed deployments**: Agents without management keys will not encrypt, breaking topology privacy

## Environment Variables

```yaml
management:
  public_key: "${MANAGEMENT_PUBLIC_KEY}"
  private_key: "${MANAGEMENT_PRIVATE_KEY}"  # Only on management nodes
```

## Related

- [Security Overview](/security/overview) - Security architecture
- [Deployment Scenarios](/deployment/scenarios) - Deployment patterns
