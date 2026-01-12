---
title: Getting Started Overview
sidebar_position: 1
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-presenting.png" alt="Mole presenting" style={{maxWidth: '180px'}} />
</div>

# Getting Started

In the next few minutes, you will have a working tunnel that can reach networks behind firewalls, relay traffic through multiple hops, and route connections to any destination.

**What you will build:**
- A SOCKS5 proxy that tunnels traffic through restricted networks
- Multi-hop relay chains that reach destinations you could not access directly
- Exit points that connect to internal resources or the internet

## Prerequisites

- **Muti Metroo binary** - [Download for your platform](/download)
- **Basic networking knowledge** (TCP/IP, CIDR notation)

## Choose Your Path

Pick the setup method that fits your situation:

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

**Quick try-out:** Clone the repo and run a 4-agent demo in seconds:

```bash
git clone https://github.com/postalsys/Muti-Metroo.git
cd Muti-Metroo/examples/docker-tryout
docker compose up -d
# Access dashboard: http://localhost:18080/ui/
# Test proxy: curl -x socks5h://localhost:11080 https://httpbin.org/ip
```

## What You Will Accomplish

By the end of these guides, you will be able to:

1. **Tunnel through firewalls** - Route traffic through HTTP/2 or WebSocket that blends with normal HTTPS
2. **Reach internal networks** - Access resources behind NAT or segmented networks
3. **Build relay chains** - Connect through multiple hops to reach any destination
4. **Proxy any application** - Use SOCKS5 with curl, SSH, browsers, or any TCP application
5. **Monitor your mesh** - View connections and routes in the web dashboard

## Next Steps

1. **[Installation](/getting-started/installation)** - Download binaries for your platform
2. **[Quick Start](/getting-started/quick-start)** - Manual configuration walkthrough
3. **[Interactive Setup](/getting-started/interactive-setup)** - Guided wizard
4. **[Your First Mesh](/getting-started/first-mesh)** - Connect multiple agents

## Need Help?

- Check [Troubleshooting](/troubleshooting/common-issues) for common issues
- Review [Configuration Reference](/configuration/overview) for all options
- Read [Core Concepts](/concepts/architecture) to understand the architecture
