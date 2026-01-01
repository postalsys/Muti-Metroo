---
sidebar_position: 1
title: Download
description: Download Muti Metroo binaries for your platform
---

# Download Muti Metroo

Current Version: **v1.0.7**

Download the latest Muti Metroo binary for your platform. All binaries are self-contained and require no additional dependencies.

## macOS

### Apple Silicon (M1/M2/M3)

Download the installer package for the easiest installation experience:

- [muti-metroo-darwin-arm64.pkg](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-darwin-arm64.pkg) (Installer)

Or download the standalone binary:

- [muti-metroo-darwin-arm64](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-darwin-arm64) (Binary)

### Intel

- [muti-metroo-darwin-amd64.pkg](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-darwin-amd64.pkg) (Installer)
- [muti-metroo-darwin-amd64](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-darwin-amd64) (Binary)

### macOS Installation

**Using the installer (recommended):**

1. Download the `.pkg` file for your architecture
2. Double-click to run the installer
3. Follow the installation prompts
4. The binary will be installed to `/usr/local/bin/muti-metroo`

**Using the standalone binary:**

```bash
# Download and install
curl -L -o muti-metroo https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-darwin-arm64
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Verify installation
muti-metroo --version
```

## Linux

### x86_64 (amd64)

- [muti-metroo-linux-amd64](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-linux-amd64)

### ARM64 (aarch64)

- [muti-metroo-linux-arm64](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-linux-arm64)

### Linux Installation

```bash
# Download for your architecture (example: amd64)
curl -L -o muti-metroo https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-linux-amd64

# Make executable and install
chmod +x muti-metroo
sudo mv muti-metroo /usr/local/bin/

# Verify installation
muti-metroo --version
```

## Windows

### x86_64 (amd64)

- [muti-metroo-windows-amd64.exe](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-windows-amd64.exe)

### ARM64

- [muti-metroo-windows-arm64.exe](https://muti-metroo.postalsys.ee/downloads/latest/muti-metroo-windows-arm64.exe)

### Windows Installation

1. Download the `.exe` file for your architecture
2. Move the file to a directory in your PATH (e.g., `C:\Program Files\muti-metroo\`)
3. Open Command Prompt or PowerShell and verify:

```powershell
muti-metroo.exe --version
```

## Checksums

Verify your download with SHA256 checksums:

- [checksums.txt](https://muti-metroo.postalsys.ee/downloads/latest/checksums.txt)

To verify on Linux/macOS:

```bash
# Download checksums
curl -L -o checksums.txt https://muti-metroo.postalsys.ee/downloads/latest/checksums.txt

# Verify (example for linux-amd64)
sha256sum -c checksums.txt --ignore-missing
```

To verify on Windows (PowerShell):

```powershell
# Get the expected hash from checksums.txt
$expected = (Get-Content checksums.txt | Select-String "muti-metroo-windows-amd64.exe").ToString().Split()[0]

# Calculate actual hash
$actual = (Get-FileHash muti-metroo-windows-amd64.exe -Algorithm SHA256).Hash.ToLower()

# Compare
if ($expected -eq $actual) { "OK" } else { "MISMATCH" }
```

## Previous Versions

Previous releases are available at:

- [All Releases](https://muti-metroo.postalsys.ee/downloads/)

## Next Steps

After installation, follow the [Quick Start Guide](/getting-started/quick-start) to set up your first Muti Metroo agent.
