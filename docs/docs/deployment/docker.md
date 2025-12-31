---
title: Docker Deployment
sidebar_position: 2
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-climbing.png" alt="Mole deploying to Docker" style={{maxWidth: '180px'}} />
</div>

# Docker Deployment

Deploy Muti Metroo using Docker and Docker Compose.

## Docker Image

### Build Image

```bash
# From repository root
docker build -t muti-metroo .

# With version tag
docker build -t muti-metroo:v1.0.0 .
```

### Dockerfile

The included Dockerfile uses multi-stage build:

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o muti-metroo ./cmd/muti-metroo

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/muti-metroo .
ENTRYPOINT ["./muti-metroo"]
CMD ["run", "-c", "/app/config.yaml"]
```

## Running a Single Container

### Basic Run

```bash
docker run -d --name muti-metroo \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/app/certs \
  -p 1080:1080 \
  -p 4433:4433/udp \
  -p 8080:8080 \
  muti-metroo
```

### With Environment Variables

```bash
docker run -d --name muti-metroo \
  -e LOG_LEVEL=debug \
  -e SOCKS_ADDR=0.0.0.0:1080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/app/certs \
  -p 1080:1080 \
  -p 4433:4433/udp \
  -p 8080:8080 \
  muti-metroo
```

### With Secrets

```bash
docker run -d --name muti-metroo \
  --env-file .env \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/app/certs \
  -p 1080:1080 \
  -p 4433:4433/udp \
  -p 8080:8080 \
  muti-metroo
```

## Docker Compose

### Basic Setup

```yaml
# docker-compose.yml
version: '3.8'

services:
  agent:
    build: .
    restart: unless-stopped
    ports:
      - "1080:1080"       # SOCKS5
      - "4433:4433/udp"   # QUIC
      - "8080:8080"       # HTTP API
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./data:/app/data
      - ./certs:/app/certs:ro
    environment:
      - LOG_LEVEL=info
```

### Multi-Agent Testbed

```yaml
# docker-compose.yml
version: '3.8'

services:
  # Ingress agent
  agent1:
    build: .
    restart: unless-stopped
    ports:
      - "1081:1080"
      - "8081:8080"
      - "4433:4433/udp"
    volumes:
      - ./configs/agent1.yaml:/app/config.yaml:ro
      - ./data/agent1:/app/data
      - ./certs:/app/certs:ro
    networks:
      - mesh

  # Transit agent
  agent2:
    build: .
    restart: unless-stopped
    ports:
      - "1082:1080"
      - "8082:8080"
      - "4434:4433/udp"
    volumes:
      - ./configs/agent2.yaml:/app/config.yaml:ro
      - ./data/agent2:/app/data
      - ./certs:/app/certs:ro
    networks:
      - mesh

  # Exit agent
  agent3:
    build: .
    restart: unless-stopped
    ports:
      - "1083:1080"
      - "8083:8080"
      - "4435:4433/udp"
    volumes:
      - ./configs/agent3.yaml:/app/config.yaml:ro
      - ./data/agent3:/app/data
      - ./certs:/app/certs:ro
    networks:
      - mesh

networks:
  mesh:
    driver: bridge
```

### With Health Checks

```yaml
services:
  agent:
    build: .
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

## Configuration for Docker

### Agent Config

```yaml
agent:
  id: "auto"
  display_name: "${HOSTNAME:-docker-agent}"
  data_dir: "/app/data"
  log_level: "${LOG_LEVEL:-info}"
  log_format: "json"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "/app/certs/agent.crt"
      key: "/app/certs/agent.key"

socks5:
  enabled: true
  address: "0.0.0.0:1080"

http:
  enabled: true
  address: "0.0.0.0:8080"
```

### Docker-Internal Networking

Use container names for peer addresses:

```yaml
# agent1.yaml
peers:
  - id: "${AGENT2_ID}"
    transport: quic
    address: "agent2:4433"
    tls:
      ca: "/app/certs/ca.crt"

# agent2.yaml
peers:
  - id: "${AGENT3_ID}"
    transport: quic
    address: "agent3:4433"
    tls:
      ca: "/app/certs/ca.crt"
```

## Docker Commands

### Build and Start

```bash
# Build images
docker compose build

# Start all agents
docker compose up -d

# Start specific agents
docker compose up -d agent1 agent2

# View logs
docker compose logs -f agent1

# Stop all
docker compose down
```

### Inspect and Debug

```bash
# Container status
docker compose ps

# Enter container shell
docker compose exec agent1 sh

# View agent logs
docker compose logs agent1

# Follow logs
docker compose logs -f --tail=100 agent1

# Check health
docker compose exec agent1 wget -q -O - http://localhost:8080/health
```

### Certificate Management

```bash
# Generate certs on host, mount to container
./build/muti-metroo cert ca -n "Docker Mesh CA" -o ./certs
./build/muti-metroo cert agent -n "agent1" -o ./certs
```

## Production Considerations

### Resource Limits

```yaml
services:
  agent:
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
```

### Logging

```yaml
services:
  agent:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

### Secrets Management

```yaml
services:
  agent:
    secrets:
      - tls_cert
      - tls_key
      - ca_cert

secrets:
  tls_cert:
    file: ./certs/agent.crt
  tls_key:
    file: ./certs/agent.key
  ca_cert:
    file: ./certs/ca.crt
```

### Networking

```yaml
services:
  agent:
    network_mode: host    # For better UDP performance (QUIC)
```

Or use bridge with explicit port mapping:

```yaml
services:
  agent:
    ports:
      - "4433:4433/udp"
    networks:
      - mesh
```

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker compose logs agent1

# Check config syntax
docker compose config

# Test config
docker compose run --rm agent1 ./muti-metroo validate -c /app/config.yaml
```

### Network Issues

```bash
# Check port bindings
docker compose ps

# Test internal connectivity
docker compose exec agent1 nc -zv agent2 4433

# Check DNS resolution
docker compose exec agent1 nslookup agent2
```

### Permission Issues

```bash
# Check file permissions
docker compose exec agent1 ls -la /app/

# Fix data directory permissions
docker compose exec agent1 chown -R 1000:1000 /app/data
```

## Next Steps

- [Kubernetes Deployment](kubernetes) - Deploy on K8s
- [System Service](system-service) - Native installation
- [Monitoring](../features/metrics-monitoring) - Prometheus setup
