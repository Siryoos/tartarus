# Kubernetes Deployment

Deploy Tartarus on Kubernetes using Helm.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.x
- kubectl configured

## Installation

### Add Helm Repository

```bash
helm repo add tartarus https://charts.tartarus.io
helm repo update
```

### Install the Operator

```bash
helm install tartarus-operator tartarus/tartarus-operator \
  --namespace tartarus-system \
  --create-namespace
```

## Custom Resource Definitions

### SandboxJob

```yaml
apiVersion: tartarus.io/v1alpha1
kind: SandboxJob
metadata:
  name: my-job
  namespace: default
spec:
  template: python-ds
  resources:
    cpu: 2
    memory: 4096
  timeout: 1h
  script: |
    python train.py
```

### SandboxTemplate

```yaml
apiVersion: tartarus.io/v1alpha1
kind: SandboxTemplate
metadata:
  name: python-ds
  namespace: default
spec:
  image: python:3.11-slim
  packages:
    - numpy
    - pandas
  defaultResources:
    cpu: 1
    memory: 2048
```

## Helm Values

```yaml
# values.yaml
operator:
  replicas: 2
  resources:
    limits:
      cpu: 500m
      memory: 256Mi

metrics:
  enabled: true
  serviceMonitor:
    enabled: true

rbac:
  create: true

multiTenant:
  enabled: true
  networkPolicies: true
```

## Multi-Tenancy

Enable tenant isolation with network policies:

```yaml
apiVersion: tartarus.io/v1alpha1
kind: TenantNetworkPolicy
metadata:
  name: tenant-a
spec:
  tenantID: tenant-a
  ingress:
    allowedPorts: [8080]
  egress:
    allowedDomains:
      - "*.internal.company.com"
    deniedCIDRs:
      - "10.0.0.0/8"
```

## Monitoring

The operator exposes Prometheus metrics:

```bash
kubectl port-forward -n tartarus-system svc/tartarus-operator-metrics 8080
curl http://localhost:8080/metrics
```
