---
title: Getting Started
sidebar_position: 2
---

# Getting Started

This guide will help you get Muti Metroo up and running on your system.

## Prerequisites

- Go 1.23 or later (for building from source)
- Make (optional, for convenience commands)
- TLS certificates (can be generated with `muti-metroo cert` command)

## Installation Options

You have three options for getting started with Muti Metroo:

1. **Interactive Setup Wizard** (Recommended) - Guided setup with automatic configuration
2. **Manual Setup** - Full control over configuration
3. **Docker** - Containerized deployment

Choose the method that best fits your needs:

- Use the **[Interactive Setup](interactive-setup)** for the quickest path to a working mesh
- Use **[Manual Setup](quick-start)** if you need precise control or are setting up production deployments
- Use **[Docker](development/docker)** for development, testing, or containerized environments

## Quick Installation

```bash
# Clone the repository
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4

# Build the binary
make build

# Run interactive setup
./build/muti-metroo setup
```

## Next Steps

- [Installation](installation) - Detailed installation instructions
- [Quick Start](quick-start) - Manual configuration guide
- [Interactive Setup](interactive-setup) - Using the setup wizard
- [Configuration](configuration) - Configuration file reference
