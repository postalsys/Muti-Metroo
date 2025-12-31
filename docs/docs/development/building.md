---
title: Building
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-hammering.png" alt="Mole building" style={{maxWidth: '180px'}} />
</div>

# Building Muti Metroo

How to build Muti Metroo from source.

## Prerequisites

- Go 1.23 or later
- Git
- Make (optional, for convenience commands)

## Clone Repository

```bash
git clone ssh://git@git.aiateibad.ee:3346/andris/Muti-Metroo-v4.git
cd Muti-Metroo-v4
```

## Build Commands

### Using Make

```bash
# Build binary
make build

# Build and install to $GOPATH/bin
make install

# Clean build artifacts
make clean

# Download dependencies
make deps
```

### Using Go Directly

```bash
# Build
go build -o build/muti-metroo ./cmd/muti-metroo

# Install
go install ./cmd/muti-metroo

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o build/muti-metroo-linux-amd64 ./cmd/muti-metroo
GOOS=windows GOARCH=amd64 go build -o build/muti-metroo-windows-amd64.exe ./cmd/muti-metroo
```

## Build Outputs

Binary is created at:
```
muti-metroo
```

## Cross-Compilation

Build for different platforms:

```bash
# Linux
GOOS=linux GOARCH=amd64 make build

# Windows
GOOS=windows GOARCH=amd64 make build

# macOS
GOOS=darwin GOARCH=amd64 make build

# ARM (Raspberry Pi)
GOOS=linux GOARCH=arm64 make build
```

## Build Flags

Custom build flags:

```bash
# Static binary (no CGO dependencies)
CGO_ENABLED=0 go build -o build/muti-metroo ./cmd/muti-metroo

# With debug symbols removed
go build -ldflags="-s -w" -o build/muti-metroo ./cmd/muti-metroo

# With version information
go build -ldflags="-X main.version=1.0.0" -o build/muti-metroo ./cmd/muti-metroo
```

## Development Setup

```bash
# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Generate mocks (if using)
go generate ./...

# Format code
make fmt

# Run linter
make lint
```

## IDE Setup

### VS Code

Install Go extension and use this `.vscode/settings.json`:

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "workspace",
  "editor.formatOnSave": true
}
```

### GoLand

No special configuration needed. Open the project directory.
