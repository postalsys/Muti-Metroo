---
title: Docker Development
sidebar_position: 3
---

# Docker Development

Using Docker for development and testing.

## Docker Setup

### Build Image

```bash
# Build production image
docker build -t muti-metroo .

# Build with specific tag
docker build -t muti-metroo:v1.0.0 .
```

### Run Container

```bash
# Run with config
docker run -v $(pwd)/config.yaml:/app/config.yaml            -v $(pwd)/data:/app/data            -v $(pwd)/certs:/app/certs            -p 1080:1080 -p 4433:4433/udp -p 8080:8080            muti-metroo

# Run in background
docker run -d --name muti-metroo            -v $(pwd)/config.yaml:/app/config.yaml            -v $(pwd)/data:/app/data            -v $(pwd)/certs:/app/certs            -p 1080:1080 -p 4433:4433/udp -p 8080:8080            muti-metroo

# View logs
docker logs -f muti-metroo
```

## Docker Compose

### Multi-Agent Testbed

The repository includes a Docker Compose setup for testing:

```bash
# Build all images
docker compose build

# Start 3-agent mesh
docker compose up -d agent1 agent2 agent3

# View logs
docker compose logs -f agent1

# Stop all
docker compose down
```

### docker-compose.yml Example

```yaml
services:
  agent1:
    build: .
    ports:
      - "1081:1080"
      - "8081:8080"
      - "4433:4433/udp"
    volumes:
      - ./configs/agent1.yaml:/app/config.yaml
      - ./data/agent1:/app/data
      - ./certs:/app/certs

  agent2:
    build: .
    ports:
      - "1082:1080"
      - "8082:8080"
      - "4434:4433/udp"
    volumes:
      - ./configs/agent2.yaml:/app/config.yaml
      - ./data/agent2:/app/data
      - ./certs:/app/certs

  agent3:
    build: .
    ports:
      - "1083:1080"
      - "8083:8080"
      - "4435:4433/udp"
    volumes:
      - ./configs/agent3.yaml:/app/config.yaml
      - ./data/agent3:/app/data
      - ./certs:/app/certs
```

## Development Workflow

### 1. Build and Start Testbed

```bash
docker compose build
docker compose up -d
```

### 2. Test Connectivity

```bash
# Check health
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health

# Check peers
curl http://localhost:8081/healthz
```

### 3. Test SOCKS5 Proxy

```bash
# From host machine
curl -x socks5://localhost:1081 https://example.com
```

### 4. View Metrics

```bash
curl http://localhost:8081/metrics
```

### 5. Run Tests in Container

```bash
# Run tests in isolated container
docker compose run test

# Or run tests in existing container
docker compose exec agent1 go test ./...
```

## Dockerfile

Multi-stage build for minimal image size:

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o muti-metroo ./cmd/muti-metroo

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/muti-metroo .
ENTRYPOINT ["./muti-metroo"]
CMD ["run", "-c", "/app/config.yaml"]
```

## Debugging in Docker

### Attach to Running Container

```bash
docker exec -it muti-metroo sh
```

### View Logs with Filtering

```bash
# Follow logs
docker logs -f muti-metroo

# Last 100 lines
docker logs --tail 100 muti-metroo

# Since 10m ago
docker logs --since 10m muti-metroo
```

### Health Checks

Add health check to Dockerfile:

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s   CMD wget -q --spider http://localhost:8080/health || exit 1
```

## Production Deployment

### Docker Swarm

```bash
docker stack deploy -c docker-compose.yml muti-metroo
```

### Kubernetes

See Kubernetes manifests in `k8s/` directory.

## Best Practices

1. **Use volumes** for persistent data (`./data`)
2. **Mount configs** instead of baking into image
3. **Expose metrics** for monitoring (port 8080)
4. **Use health checks** for orchestration
5. **Tag images** with version numbers
6. **Multi-stage builds** for smaller images

## Related

- [Deployment - Docker](../deployment/docker) - Production Docker deployment
- [Deployment - Kubernetes](../deployment/kubernetes) - Kubernetes deployment
- [Development - Testing](testing) - Running tests
- [Development - Building](building) - Build from source
