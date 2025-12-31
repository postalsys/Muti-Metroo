---
title: Testing
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-hammering.png" alt="Mole testing" style={{maxWidth: '180px'}} />
</div>

# Testing

How to run tests for Muti Metroo.

## Running Tests

### Using Make

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run short tests only
make test-short

# View coverage report
make test-coverage
open coverage/index.html
```

### Using Go Directly

```bash
# All tests
go test ./...

# With race detection
go test -race ./...

# Verbose output
go test -v ./...

# Specific package
go test -v ./internal/transport/...

# Single test
go test -v -run TestName ./internal/peer/

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Test Categories

### Unit Tests

Test individual packages:

```bash
go test ./internal/protocol/...
go test ./internal/routing/...
go test ./internal/stream/...
```

### Integration Tests

Test multi-agent scenarios:

```bash
go test -v ./internal/integration/...
```

Integration tests start multiple agents and verify:
- Peer connections
- Route propagation
- Stream forwarding
- SOCKS5 proxy
- Exit routing

### Load Tests

Performance and stress testing:

```bash
go test -v ./internal/loadtest/...
```

Load tests verify:
- Stream throughput
- Route table performance
- Connection churn
- Memory usage

## Writing Tests

### Unit Test Example

```go
func TestRouteMatch(t *testing.T) {
    rt := routing.NewRouteTable()
    
    err := rt.AddRoute(routing.Route{
        CIDR:   "10.0.0.0/8",
        NextHop: agentID,
        Metric:  1,
    })
    require.NoError(t, err)
    
    route, err := rt.Lookup(net.ParseIP("10.1.2.3"))
    require.NoError(t, err)
    assert.Equal(t, agentID, route.NextHop)
}
```

### Integration Test Example

```go
func TestEndToEnd(t *testing.T) {
    // Start agents
    ingress := startAgent(t, ingressConfig)
    exit := startAgent(t, exitConfig)
    
    // Wait for connection
    waitForPeers(t, ingress, 1)
    
    // Test SOCKS5 connection
    conn, err := net.Dial("tcp", "localhost:1080")
    require.NoError(t, err)
    defer conn.Close()
    
    // Verify connection through mesh
    // ...
}
```

## Test Utilities

Helper functions in `internal/` packages:

- `integration.StartAgent()` - Start test agent
- `integration.WaitForPeers()` - Wait for peer connections
- `loadtest.StreamThroughput()` - Measure throughput
- `chaos.InjectFault()` - Fault injection

## Continuous Integration

Tests run on every commit via CI/CD:

```yaml
# .github/workflows/test.yml
- name: Run tests
  run: |
    make test
    make test-coverage
```

## Benchmarking

Run benchmarks:

```bash
# All benchmarks
go test -bench=. ./...

# Specific benchmark
go test -bench=BenchmarkFrameEncode ./internal/protocol/

# With memory profiling
go test -bench=. -benchmem ./...

# CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof
```

## Coverage Goals

Target coverage by package:
- Core packages (protocol, routing, stream): >80%
- Feature packages (socks5, exit, rpc): >70%
- Integration tests: Critical paths covered
