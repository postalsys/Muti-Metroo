---
title: Getting Started Overview
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting" style={{maxWidth: '180px'}} />
</div>

# Getting Started

Welcome to Muti Metroo! This guide will help you get up and running with your first mesh network.

## Prerequisites

Before you begin, ensure you have:

- **Muti Metroo binary** - [Download for your platform](/download)
- **Basic networking knowledge** (understanding of TCP/IP, CIDR notation)

## Choose Your Path

Depending on your goals, choose the best starting point:

### Quick Evaluation

Want to quickly try Muti Metroo? Use the **Interactive Setup Wizard**:

```bash
muti-metroo setup
```

The wizard guides you through all configuration steps with sensible defaults.

**Time: 5-10 minutes**

[Go to Interactive Setup](/getting-started/interactive-setup)

### Manual Configuration

Need full control over your configuration? Follow the **Quick Start Guide**:

```bash
# Initialize agent and create config manually
muti-metroo init -d ./data
# Edit config.yaml
muti-metroo run -c ./config.yaml
```

**Time: 15-30 minutes**

[Go to Quick Start](/getting-started/quick-start)

### Docker Deployment

For containerized deployments, see the **Docker guide**:

[Go to Docker Deployment](/deployment/docker)

## What You Will Learn

By the end of the Getting Started guides, you will:

1. Have Muti Metroo installed and running
2. Understand the basic agent roles (ingress, transit, exit)
3. Have generated TLS certificates for secure communication
4. Created a basic configuration file
5. Connected two or more agents into a mesh
6. Successfully proxied traffic through your mesh

## Next Steps

1. **[Installation](/getting-started/installation)** - Download binaries for your platform
2. **[Quick Start](/getting-started/quick-start)** - Manual configuration walkthrough
3. **[Interactive Setup](/getting-started/interactive-setup)** - Guided wizard
4. **[Your First Mesh](/getting-started/first-mesh)** - Connect multiple agents

## Need Help?

- Check [Troubleshooting](/troubleshooting/common-issues) for common issues
- Review [Configuration Reference](/configuration/overview) for all options
- Read [Core Concepts](/concepts/architecture) to understand the architecture
