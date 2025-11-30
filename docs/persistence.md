# Olympus Persistence Configuration

Tartarus Olympus (the control plane API) supports pluggable persistence backends. For production deployments, **Redis** is required to ensure state durability and high availability.

## Quick Start (Development)

The included `docker-compose.yml` already configures Redis by default - just run:

```bash
docker-compose up --build
```

This automatically sets `REDIS_ADDR=redis:6379` for both Olympus and the agent, ensuring persistence works out of the box.

## Production Configuration

When running in production mode (`TARTARUS_ENV=production`), Olympus enforces the presence of Redis configuration. If Redis is not configured, the service will fail to start.

> [!IMPORTANT]
> **Always use Redis for production deployments.** In-memory storage is only suitable for local development and testing.

### Required Environment Variables

| Variable | Description | Required in Production | Default |
|----------|-------------|------------------------|------------|
| `TARTARUS_ENV` | Environment mode (`development` or `production`) | No | `development` |
| `REDIS_ADDR` | Redis server address (e.g., `localhost:6379`) | **Yes** (if env is prod) | - |
| `REDIS_DB` | Redis database number | No | `0` |
| `REDIS_QUEUE_KEY` | Redis key for the work queue | No | `tartarus:queue` |
| `REDIS_PASSWORD` | Redis password (optional) | No | - |

### Behavior

- **Development (`TARTARUS_ENV=development` or unset)**:
  - If `REDIS_ADDR` is set, Redis is used.
  - If `REDIS_ADDR` is unset, in-memory storage is used. **State will be lost on restart.**

- **Production (`TARTARUS_ENV=production`)**:
  - `REDIS_ADDR` **MUST** be set.
  - If unset, Olympus exits with a fatal error.
  - Ensures that all sandbox state (Runs) and the work queue are persisted to Redis.

## Persistence Guarantees

- **Sandbox Runs**: Persisted in Redis via `Hades` registry. Survives Olympus restarts.
- **Work Queue**: Persisted in Redis via `Acheron` queue. Pending tasks survive restarts.
- **Node Registry**: Node heartbeats are stored in Redis with TTL.

## Themis Policy Persistence

Themis policies are persisted to ensure they survive control-plane restarts.

### Configuration

Themis uses the same Redis instance as Olympus.

- **Storage Key**: Policies are stored under `themis:policies`.
- **Versioning**: Each policy update increments a version counter.
- **Durability**: Policies are loaded from Redis on startup. If Redis is unavailable in production, startup will fail.

### Policy Lifecycle

1. **Creation/Update**: Policies are written to Redis and broadcast to all Olympus instances.
2. **Retrieval**: Policies are read from Redis with a local in-memory cache for performance.
3. **Enforcement**: Rhadamanthus uses the cached policies for admission control.

## Verifying Persistence

To verify that persistence is working correctly:

1. Start Olympus with Redis configured
2. Submit a sandbox request
3. Restart Olympus
4. List sandboxes - your previous request should still be visible

Automated test: `go test -v ./pkg/olympus -run TestOlympusPersistence_RestartRecovery`

## Troubleshooting

### "Using in-memory queue" in production

**Problem**: Olympus is using in-memory storage despite Redis being available.

**Solution**: Set `TARTARUS_ENV=production` to enforce Redis requirement, or explicitly set `REDIS_ADDR` environment variable.

### State lost after restart

**Problem**: Sandbox requests disappear after Olympus restarts.

**Solution**: Verify that `REDIS_ADDR` is configured and Olympus logs show "Using Redis queue" and "Using Redis registry" on startup.

### Connection refused errors

**Problem**: Cannot connect to Redis.

**Solution**: 
- Verify Redis is running: `redis-cli ping`
- Check `REDIS_ADDR` format (should be `host:port`, not `redis://host:port`)
- Ensure network connectivity between Olympus and Redis
