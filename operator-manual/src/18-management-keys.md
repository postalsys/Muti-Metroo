# Management Key Encryption

Management key encryption provides cryptographic compartmentalization of mesh topology information. When enabled, sensitive node information (hostnames, OS, IPs, peer lists) is encrypted so only operators with the private key can view topology details.

## Threat Model

### Protected Against

- Blue team captures agent, enables dashboard - sees encrypted blobs only
- Blue team dumps agent memory - no private key present
- Blue team analyzes network traffic - NodeInfo encrypted
- Compromised field agent - cannot expose other agents' details

### Not Protected Against

- Traffic analysis (connection patterns visible)
- Agent ID correlation (IDs remain plaintext for routing)
- Compromise of operator machine with private key

## Key Generation

Generate a management keypair:

```bash
muti-metroo management-key generate
```

Output:

```
Management Keypair Generated
============================
Public Key:  a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd
Private Key: e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef

IMPORTANT: Store the private key securely!
```

## Deployment Configuration

### Field Agents (Encrypt Only)

Field agents get the **public key only** - they can encrypt but not decrypt:

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  # NO private_key - field agents cannot decrypt
```

### Operator Nodes (Can Decrypt)

Operator stations get **both keys** - they can decrypt the topology:

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"
```

## What Gets Protected

| Data | Encrypted | Plaintext | Reason |
|------|:---------:|:---------:|--------|
| Hostname, OS, IPs | Yes | | System identification |
| Peer list | Yes | | Topology exposure |
| Agent display name | Yes | | Operational naming |
| Agent IDs | | Yes | Required for routing |
| Route CIDRs/metrics | | Yes | Required for routing |
| Stream data | N/A | N/A | Has its own E2E encryption |

## API Behavior

### Without Private Key (Field Agent)

When accessing dashboard APIs on a field agent:

```bash
curl http://field-agent:8080/api/dashboard
```

Response:

```json
{
  "agent": { "display_name": "local-only", "is_local": true },
  "peers": [],
  "routes": []
}
```

```bash
curl http://field-agent:8080/api/nodes
```

Response:

```json
{
  "nodes": [
    { "is_local": true }
  ]
}
```

### With Private Key (Operator)

When accessing from operator station:

```bash
curl http://operator:8080/api/dashboard
```

Response includes full decrypted topology:

```json
{
  "agent": { "display_name": "Gateway", "hostname": "gw-01", ... },
  "peers": [
    { "agent_id": "...", "display_name": "Field-1", ... }
  ],
  "routes": [...]
}
```

## Deployment Example

### Step 1: Generate Keys

On a secure machine:

```bash
muti-metroo management-key generate > keys.txt
cat keys.txt
```

### Step 2: Deploy Field Agents

Copy only the public key to field agent configs:

```yaml
# field-agent-config.yaml
agent:
  display_name: "Field Agent 1"

management:
  public_key: "a1b2c3d4..."
```

### Step 3: Configure Operator Station

Include both keys on operator machines:

```yaml
# operator-config.yaml
agent:
  display_name: "Operator Console"

management:
  public_key: "a1b2c3d4..."
  private_key: "e5f6a7b8..."
```

### Step 4: Verify

On field agent (should show limited info):

```bash
curl http://localhost:8080/api/nodes | jq '.nodes | length'
# Output: 1 (only local node)
```

On operator (should show full topology):

```bash
curl http://localhost:8080/api/nodes | jq '.nodes | length'
# Output: 5 (all nodes visible)
```

## Key Management Best Practices

1. **Generate keys offline** on a secure machine
2. **Never store private keys** on field agents
3. **Limit operator access** to authorized personnel only
4. **Rotate keys periodically** for long-running operations
5. **Destroy keys** after operation concludes

## Using Environment Variables

For additional security, pass keys via environment:

```yaml
management:
  public_key: "${MGMT_PUBKEY}"
  private_key: "${MGMT_PRIVKEY}"
```

```bash
export MGMT_PUBKEY="a1b2c3d4..."
export MGMT_PRIVKEY="e5f6a7b8..."
muti-metroo run -c config.yaml
```
