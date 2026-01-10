---
title: management-key
sidebar_position: 10
---

# management-key

Hide your mesh topology from field agents. Generate encryption keys that let operators see which systems are in the mesh while field agents only see opaque IDs.

**What this protects:** If a field agent is compromised, the attacker learns nothing about your infrastructure - no hostnames, no IP addresses, no OS details. They see only random-looking agent IDs.

**Quick setup:**
```bash
# Generate a keypair
muti-metroo management-key generate

# Add public key to ALL agents (they can encrypt but not decrypt)
# Add private key ONLY to operator nodes (they can see topology)
```

## Subcommands

### generate

Generate a new management keypair:

```bash
muti-metroo management-key generate
```

**Output:**
```
=== Management Keypair Generated ===

Public Key (add to ALL agent configs):
  a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd

Private Key (add ONLY to operator configs - KEEP SECRET!):
  e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef

Configuration snippets:

For ALL agents (field agents and operators):
  management:
    public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"

For OPERATOR nodes only (add private key):
  management:
    public_key: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd"
    private_key: "e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef"
```

### public

Derive public key from a private key:

```bash
muti-metroo management-key public --private <private-key-hex>
```

**Flags:**
- `--private, -p` (required): Private key in hex format (64 characters)

**Example:**
```bash
muti-metroo management-key public --private e5f6a7b8c9d012345678901234567890123456789012345678901234567890ef
```

**Output:**
```
Public Key: a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd
```

## Usage Guide

### Initial Setup

1. **Generate keypair** on a secure operator machine:
   ```bash
   muti-metroo management-key generate
   ```

2. **Save the private key** securely (password manager, encrypted file)

3. **Distribute public key** to all agents via config

4. **Add private key** only to operator nodes that need topology visibility

### Configuration

**Field agents** (encrypt only - cannot view topology):
```yaml
management:
  public_key: "a1b2c3d4..."
```

**Operator nodes** (can encrypt and decrypt):
```yaml
management:
  public_key: "a1b2c3d4..."
  private_key: "e5f6a7b8..."
```

### Security Model

| Role | Has Public Key | Has Private Key | Can View Topology |
|------|---------------|-----------------|-------------------|
| Field Agent | Yes | No | No |
| Operator | Yes | Yes | Yes |

### What Gets Protected

- **Encrypted**: NodeInfo (hostname, OS, IP addresses, peer lists)
- **Plaintext**: Agent IDs, route CIDRs (required for routing)

Without the private key, agents see only opaque 128-bit agent IDs instead of meaningful system identification.

## Related

- [Management Keys](/red-team/management-keys)
- [Configuration Overview](/configuration/overview)
