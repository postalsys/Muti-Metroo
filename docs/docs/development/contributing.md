---
title: Contributing
sidebar_position: 4
---

# Contributing

Guidelines for contributing to Muti Metroo.

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/yourusername/muti-metroo.git
   cd muti-metroo
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/original/muti-metroo.git
   ```

## Development Setup

### Prerequisites

- Go 1.23 or later
- Docker and Docker Compose (recommended)
- Make

### Build from Source

```bash
# Build binary
make build

# Run tests
make test

# Run linter
make lint
```

### Using Docker (Recommended)

```bash
# Build all images
docker compose build

# Start testbed
docker compose up -d agent1 agent2 agent3

# Run tests in container
docker compose run test
```

## Code Style

### General Guidelines

1. **ASCII only**: No emojis or non-ASCII characters in code, comments, or documentation
2. Follow standard Go conventions
3. Keep functions focused and small
4. Write descriptive variable and function names
5. Add comments for non-obvious logic

### Formatting

```bash
# Format code
make fmt

# Or directly
gofmt -w .
```

### Linting

```bash
# Run linter
make lint

# Fix issues before committing
go vet ./...
```

## Testing

### Running Tests

```bash
# All tests with race detection
make test

# Short tests only
make test-short

# Specific package
go test -v ./internal/peer/

# Single test
go test -v -run TestPeerConnection ./internal/peer/
```

### Writing Tests

1. Place tests in `*_test.go` files
2. Use table-driven tests for multiple cases
3. Test both success and error paths
4. Use meaningful test names

```go
func TestRouteTable_LongestPrefixMatch(t *testing.T) {
    tests := []struct {
        name     string
        routes   []Route
        dest     string
        wantCIDR string
    }{
        {
            name:     "exact match",
            routes:   []Route{{CIDR: "10.0.0.1/32"}},
            dest:     "10.0.0.1",
            wantCIDR: "10.0.0.1/32",
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests

Integration tests are in `internal/integration/`:

```bash
# Run integration tests
go test -v ./internal/integration/

# With Docker testbed
docker compose up -d agent1 agent2 agent3
go test -v ./internal/integration/ -tags=integration
```

## Pull Request Process

### Before Submitting

1. **Sync with upstream**:
   ```bash
   git fetch upstream
   git rebase upstream/master
   ```

2. **Run all checks**:
   ```bash
   make lint
   make test
   ```

3. **Update documentation** if needed

### PR Guidelines

1. **One feature per PR**: Keep PRs focused
2. **Descriptive title**: Summarize the change
3. **Description**: Explain what and why
4. **Link issues**: Reference related issues
5. **Add tests**: Cover new functionality

### Commit Messages

Format:
```
<type>: <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance

Examples:
```
feat: Add WebSocket transport support

Implements WebSocket transport for traversing HTTP proxies.
Includes connection upgrade handling and frame wrapping.

Closes #123
```

```
fix: Resolve race condition in stream cleanup

The stream map was being modified during iteration,
causing occasional panics under high load.

Fixes #456
```

## Code Review

### What We Look For

1. **Correctness**: Does it work as intended?
2. **Performance**: Any obvious bottlenecks?
3. **Security**: Any vulnerabilities introduced?
4. **Readability**: Is the code clear?
5. **Testing**: Adequate test coverage?
6. **Documentation**: Are changes documented?

### Responding to Feedback

- Address all comments before requesting re-review
- Ask for clarification if needed
- Be open to suggestions

## Documentation

### Where to Document

- **Code comments**: For implementation details
- **README.md**: High-level overview
- **CLAUDE.md**: Developer reference
- **docs/**: User documentation

### Documentation Style

1. Clear and concise
2. Include examples
3. Keep up to date with code changes
4. ASCII only (no emojis)

## Project Structure

```
muti-metroo/
|-- cmd/muti-metroo/     # Main entry point
|-- internal/            # Internal packages
|   |-- agent/           # Core agent logic
|   |-- config/          # Configuration
|   |-- peer/            # Peer connections
|   |-- protocol/        # Wire protocol
|   |-- routing/         # Route table
|   |-- socks5/          # SOCKS5 server
|   |-- stream/          # Stream management
|   |-- transport/       # Transport layer
|   `-- ...
|-- configs/             # Example configs
|-- docs/                # Documentation
|-- scripts/             # Build scripts
`-- Makefile             # Build commands
```

## Release Process

1. Update version in relevant files
2. Update CHANGELOG.md
3. Create release tag
4. Build release binaries
5. Publish release

## Getting Help

- Check existing issues for similar problems
- Open a new issue with:
  - Clear description
  - Steps to reproduce
  - Expected vs actual behavior
  - Relevant logs

## License

By contributing, you agree that your contributions will be licensed under the project's license.

## Related

- [Development - Building](building) - Build instructions
- [Development - Testing](testing) - Test guidelines
- [Development - Docker](docker-dev) - Docker development
