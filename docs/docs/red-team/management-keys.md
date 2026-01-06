---
title: Management Keys
sidebar_label: Management Keys
sidebar_position: 5
---

# Management Key Encryption

Management key encryption provides cryptographic compartmentalization. When enabled, NodeInfo (hostnames, OS, IPs, peer lists) is encrypted so only operators can view topology.

## Threat Model

**Protected against:**
- Blue team captures agent, enables dashboard → sees encrypted blobs only
- Blue team dumps agent memory → no private key present
- Blue team analyzes network traffic → NodeInfo encrypted
- Compromised field agent → cannot expose other agents' details

**Not protected against:**
- Traffic analysis (connection patterns visible)
- Agent ID correlation (IDs remain plaintext for routing)
- Compromise of operator machine with private key

## Key Generation

```bash
muti-metroo management-key generate
```

Output:
```
Management Keypair Generated
============================
Public Key:  a1b2c3d4e5f6... (64 hex chars)
Private Key: e5f6a7b8c9d0... (64 hex chars)

IMPORTANT: Store the private key securely!
```

## Deployment Configuration

**All field agents (encrypt only):**

```yaml
management:
  public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
  # NO private_key - field agents cannot decrypt
```

**Operator nodes (can decrypt):**

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

## API Behavior Without Private Key

When accessing dashboard APIs on a field agent (no private key):

```json
// GET /api/dashboard
{
  "agent": { "display_name": "local-only", "is_local": true },
  "peers": [],     // Empty - no peer info exposed
  "routes": []     // Empty - no route info exposed
}

// GET /api/nodes
{
  "nodes": [
    { "is_local": true }  // Only local node visible
  ]
}
```
