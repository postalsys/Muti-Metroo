---
title: Kubernetes Deployment
sidebar_position: 3
---

<div style={{textAlign: 'center', marginBottom: '2rem'}}>
  <img src="/img/mole-climbing.png" alt="Mole deploying to Kubernetes" style={{maxWidth: '180px'}} />
</div>

# Kubernetes Deployment

Deploy Muti Metroo on Kubernetes.

## Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- Docker registry (for images)

## Container Image

### Create Dockerfile

Create a Dockerfile using the pre-built binary (see [Docker Deployment](/deployment/docker) for the Dockerfile):

```dockerfile
FROM alpine:latest
RUN apk --no-cache add ca-certificates wget

ARG TARGETARCH
RUN wget -O /usr/local/bin/muti-metroo \
    https://mutimetroo.com/downloads/muti-metroo/muti-metroo-linux-${TARGETARCH} && \
    chmod +x /usr/local/bin/muti-metroo

WORKDIR /app
ENTRYPOINT ["muti-metroo"]
CMD ["run", "-c", "/app/config.yaml"]
```

### Build and Push

```bash
# Build image
docker build -t your-registry.com/muti-metroo:v1.0.0 .

# Push to registry
docker push your-registry.com/muti-metroo:v1.0.0
```

## Basic Deployment

### Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: muti-metroo
```

### ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muti-metroo-config
  namespace: muti-metroo
data:
  config.yaml: |
    agent:
      id: "auto"
      display_name: "k8s-agent"
      data_dir: "/app/data"
      log_level: "info"
      log_format: "json"

    tls:
      ca_pem: "${TLS_CA}"
      cert_pem: "${TLS_CERT}"
      key_pem: "${TLS_KEY}"
      mtls: true

    listeners:
      - transport: quic
        address: "0.0.0.0:4433"

    socks5:
      enabled: true
      address: "0.0.0.0:1080"

    http:
      enabled: true
      address: "0.0.0.0:8080"

    exit:
      enabled: true
      routes:
        - "0.0.0.0/0"
      dns:
        servers:
          - "8.8.8.8:53"
```

### Secret

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: muti-metroo-tls
  namespace: muti-metroo
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
  ca.crt: <base64-encoded-ca>
```

Create secret from files:

```bash
kubectl create secret generic muti-metroo-tls \
  --from-file=tls.crt=./certs/agent.crt \
  --from-file=tls.key=./certs/agent.key \
  --from-file=ca.crt=./certs/ca.crt \
  -n muti-metroo
```

### Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muti-metroo
  namespace: muti-metroo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: muti-metroo
  template:
    metadata:
      labels:
        app: muti-metroo
    spec:
      containers:
        - name: muti-metroo
          image: your-registry.com/muti-metroo:v1.0.0
          ports:
            - name: socks5
              containerPort: 1080
              protocol: TCP
            - name: quic
              containerPort: 4433
              protocol: UDP
            - name: http
              containerPort: 8080
              protocol: TCP
          env:
            - name: TLS_CA
              valueFrom:
                secretKeyRef:
                  name: muti-metroo-tls
                  key: ca.crt
            - name: TLS_CERT
              valueFrom:
                secretKeyRef:
                  name: muti-metroo-tls
                  key: tls.crt
            - name: TLS_KEY
              valueFrom:
                secretKeyRef:
                  name: muti-metroo-tls
                  key: tls.key
          volumeMounts:
            - name: config
              mountPath: /app/config.yaml
              subPath: config.yaml
            - name: data
              mountPath: /app/data
          resources:
            limits:
              cpu: "1"
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 128Mi
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: config
          configMap:
            name: muti-metroo-config
        - name: data
          emptyDir: {}
```

### Service

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: muti-metroo
  namespace: muti-metroo
spec:
  selector:
    app: muti-metroo
  ports:
    - name: socks5
      port: 1080
      targetPort: 1080
      protocol: TCP
    - name: quic
      port: 4433
      targetPort: 4433
      protocol: UDP
    - name: http
      port: 8080
      targetPort: 8080
      protocol: TCP
  type: LoadBalancer
```

## Apply Resources

```bash
# Apply all resources
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml

# Check status
kubectl get all -n muti-metroo

# View logs
kubectl logs -f deployment/muti-metroo -n muti-metroo
```

## Advanced Configuration

### StatefulSet (Persistent Identity)

For consistent agent IDs:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: muti-metroo
  namespace: muti-metroo
spec:
  serviceName: muti-metroo
  replicas: 1
  selector:
    matchLabels:
      app: muti-metroo
  template:
    metadata:
      labels:
        app: muti-metroo
    spec:
      containers:
        - name: muti-metroo
          image: your-registry.com/muti-metroo:v1.0.0
          volumeMounts:
            - name: data
              mountPath: /app/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 100Mi
```

### Pod Security

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  containers:
    - name: muti-metroo
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop:
            - ALL
```

### Network Policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: muti-metroo
  namespace: muti-metroo
spec:
  podSelector:
    matchLabels:
      app: muti-metroo
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - ports:
        - port: 1080
        - port: 4433
          protocol: UDP
        - port: 8080
  egress:
    - {} # Allow all egress (for exit traffic)
```

## Helm Chart

### values.yaml

```yaml
# values.yaml
replicaCount: 1

image:
  repository: your-registry.com/muti-metroo
  tag: v1.0.0
  pullPolicy: IfNotPresent

config:
  logLevel: info
  socks5:
    enabled: true
    address: "0.0.0.0:1080"
  exit:
    enabled: true
    routes:
      - "0.0.0.0/0"

tls:
  secretName: muti-metroo-tls

service:
  type: LoadBalancer
  ports:
    socks5: 1080
    quic: 4433
    http: 8080

resources:
  limits:
    cpu: 1
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

```

## High Availability

### Multiple Replicas

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muti-metroo
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
```

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: muti-metroo
  namespace: muti-metroo
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: muti-metroo
```

### Anti-Affinity

```yaml
spec:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app: muti-metroo
            topologyKey: kubernetes.io/hostname
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -l app=muti-metroo -n muti-metroo

# Check events
kubectl get events -n muti-metroo --sort-by='.lastTimestamp'

# Check logs
kubectl logs -l app=muti-metroo -n muti-metroo --previous
```

### Network Issues

```bash
# Test service connectivity
kubectl run test --rm -it --image=alpine -- sh
wget -q -O - http://muti-metroo.muti-metroo:8080/health

# Check endpoints
kubectl get endpoints muti-metroo -n muti-metroo
```

### Configuration Issues

```bash
# Verify ConfigMap
kubectl get configmap muti-metroo-config -n muti-metroo -o yaml

# Check env vars in pod
kubectl exec deployment/muti-metroo -n muti-metroo -- env | grep -i tls
```

## Next Steps

- [System Service](/deployment/system-service) - Native installation
- [High Availability](/deployment/high-availability) - Redundancy patterns
