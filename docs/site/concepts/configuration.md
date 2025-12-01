# Tartarus Configuration Reference

This document provides a comprehensive reference for configuring Tartarus in production and development environments.

## Environment Configuration

### Olympus API Configuration

| Variable | Description | Required | Default | Example |
|----------|-------------|----------|---------|---------|
| `TARTARUS_ENV` | Environment mode | No | `development` | `production` |
| `OLYMPUS_LISTEN_ADDR` | API server listen address | No | `:8080` | `:8080` |
| `REDIS_ADDR` | Redis server address | **Yes** (in production) | `localhost:6379` | `redis.example.com:6379` |
| `REDIS_DB` | Redis database number | No | `0` | `1` |
| `REDIS_PASSWORD` | Redis password | No | - | `secret123` |
| `REDIS_QUEUE_KEY` | Queue storage key prefix | No | `tartarus:queue` | `prod:queue` |
| `ENABLE_HYPNOS` | Enable Hypnos hibernation | No | `false` | `true` |

### Agent Configuration

| Variable | Description | Required | Default | Example |
|----------|-------------|----------|---------|---------|
| `OLYMPUS_URL` | Olympus API endpoint | **Yes** | - | `http://olympus:8080` |
| `AGENT_ID` | Unique agent identifier | No | Auto-generated | `agent-001` |
| `KERNEL_PATH` | Path to guest kernel | **Yes** | - | `/data/vmlinux` |
| `ROOTFS_PATH` | Path to base rootfs | **Yes** | - | `/data/rootfs.ext4` |

## Production Requirements

> [!IMPORTANT]
> For production deployments (`TARTARUS_ENV=production`), the following configurations are **mandatory**:

### 1. Redis Persistence

**Redis must be configured** for all production deployments to ensure state durability:

```bash
export TARTARUS_ENV=production
export REDIS_ADDR="redis.example.com:6379"
export REDIS_PASSWORD="your-secure-password"
```

If `REDIS_ADDR` is not set in production mode, Olympus will **fail to start**.

### 2. Node Labels for Routing

Label your nodes according to their capabilities and security requirements:

#### Standard Compute Nodes
```bash
# No special labels required for standard workloads
```

#### High-Compute Nodes (Phlegethon)
```bash
# Label nodes with high CPU/memory for hot workloads
tartarus node label <node-id> tartarus.io/phlegethon=true
```

#### Quarantine Nodes (Typhon)
```bash
# Label isolated nodes for quarantine workloads
tartarus node label <node-id> tartarus.io/typhon=true
```

#### Trusted Workload Nodes (Elysium)
```bash
# Label trusted nodes for sensitive workloads
tartarus node label <node-id> tartarus.io/elysium=true
```

> [!WARNING]
> **Quarantine nodes are required** if your policies can return `VerdictQuarantine`. Without at least one `tartarus.io/typhon=true` labeled node, quarantine requests will be **rejected**.

## Feature Flags

### Phase 4 Components

#### Hypnos (VM Hibernation)

Disabled by default in v1.0. To enable VM hibernation and sleep cycles:

```bash
export ENABLE_HYPNOS=true
```

**Status**: Implemented but not fully tested for production. Enable at your own risk.

#### Thanatos (Graceful Termination)

**Always enabled** as of Phase 6. Provides graceful shutdown, grace-period enforcement, and optional checkpoint-on-terminate via Hypnos integration.

**Status**: Production-ready. Fully tested and integrated.

> [!CAUTION]
> Enabling Hypnos in v1.0 is **not recommended** for production. This feature will be fully validated and enabled by default in Phase 4.

## Policy Configuration

### Themis Policies

Policies are stored in Redis under the `themis:policies` key with versioning support.

#### Example Policy YAML

```yaml
id: production-default
name: Production Default Policy
version: 1
network:
  deny_private: true
  deny_metadata: true
  allowed_egress_ports: [80, 443]
resources:
  max_cpu_cores: 4
  max_memory_mb: 8192
  max_duration: 5m
enforcement:
  on_timeout: kill
  on_resource_violation: throttle
  on_network_violation: quarantine
```

#### Loading Policies

Policies are loaded from Redis on Olympus startup. To add or update a policy:

```bash
# Using the Olympus API
curl -X POST http://olympus:8080/v1/policies \
  -H "Content-Type: application/json" \
  -d @policy.json
```

### Policy Versioning

- Each policy update increments the version counter
- Optimistic concurrency control prevents lost updates
- Version mismatches return `409 Conflict`

## Heat-Aware Routing (Phlegethon)

Phlegethon automatically classifies workloads by resource intensity:

| Heat Level | Criteria | Target Nodes |
|------------|----------|--------------|
| `cold` | CPU < 1 core, Memory < 1GB, Duration < 30s | Any node |
| `warm` | CPU 1-2 cores, Memory 1-4GB | Any node |
| `hot` | CPU > 2 cores or Memory > 4GB | `tartarus.io/phlegethon=true` |
| `inferno` | GPU requested or Duration > 1h | `tartarus.io/phlegethon=true` |

### Configuration

No special configuration required. Phlegethon classification is automatic based on request parameters.

To designate high-compute nodes:

```bash
tartarus node label <node-id> tartarus.io/phlegethon=true
```

### Fallback Behavior

If no `tartarus.io/phlegethon=true` nodes are available, hot workloads will fall back to standard nodes (with a warning logged).

## Quarantine Enforcement (Typhon)

Typhon provides strong isolation for suspicious or untrusted workloads.

### Triggers

Workloads are quarantined when:
1. A judge returns `VerdictQuarantine`
2. Network policy violations are detected
3. Manual quarantine flag is set: `metadata["quarantine"] = "true"`

### Required Configuration

```bash
# Designate at least one quarantine node
tartarus node label <node-id> tartarus.io/typhon=true
```

### Behavior

- **With quarantine nodes**: Jobs route exclusively to labeled nodes
- **Without quarantine nodes**: Quarantine requests are **rejected**

## Audit Logging (Aeacus)

Aeacus judge tags all requests for audit and compliance.

### Audit Metadata

Every sandbox request receives audit tags:
- `audit.request_id`: Unique request identifier
- `audit.timestamp`: Request timestamp (RFC3339)
- `audit.caller`: Caller identity (if authenticated)
- `audit.verdict`: Admission decision

### Audit Sink

Configure audit log destination:

```bash
# Future: Configure audit sink (not yet implemented)
# export AUDIT_SINK_TYPE=elasticsearch
# export AUDIT_SINK_URL=https://es.example.com
```

Currently, audit metadata is attached to requests and logged via Hermes.

## Verification

### Verify Redis Connection

```bash
# Check Olympus logs for Redis confirmation
docker logs olympus-api 2>&1 | grep "Redis"
# Expected: "Using Redis queue" and "Using Redis registry"
```

### Verify Node Labels

```bash
# List nodes with labels
curl http://olympus:8080/v1/nodes | jq '.[] | {id, labels}'
```

### Verify Policies

```bash
# List loaded policies
curl http://olympus:8080/v1/policies | jq '.'
```

## Troubleshooting

### Common Issues

#### "Redis required for production" error

**Problem**: Olympus fails to start with `TARTARUS_ENV=production` but no Redis configured.

**Solution**: Set `REDIS_ADDR` environment variable:
```bash
export REDIS_ADDR="redis:6379"
```

#### Quarantine requests rejected

**Problem**: Requests with quarantine verdict are being rejected.

**Solution**: Label at least one node for quarantine:
```bash
tartarus node label <node-id> tartarus.io/typhon=true
```

#### State lost after restart

**Problem**: Sandbox state disappears after Olympus restart.

**Solution**: Verify Redis is configured and Olympus logs show "Using Redis registry".

## Development vs. Production

| Aspect | Development | Production |
|--------|-------------|------------|
| Redis | Optional (defaults to `localhost:6379`) | **Required** |
| Hypnos | Can be enabled for testing | **Disabled** (default) |
| Thanatos | Always enabled | Always enabled |
| Node labels | Optional | **Required** for Typhon |
| Audit logging | Logged only | Should export to sink |
| Policies | In-memory cache OK | Must persist to Redis |

---

For additional information, see:
- [docs/persistence.md](file:///Users/mohammadrezayousefiha/tartarus/docs/persistence.md) - Detailed persistence configuration
- [docs/DEV_STACK.md](file:///Users/mohammadrezayousefiha/tartarus/docs/DEV_STACK.md) - Development environment setup
- [ROADMAP.md](file:///Users/mohammadrezayousefiha/tartarus/ROADMAP.md) - Phase completion status and component details
