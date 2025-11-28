# Olympus Persistence Configuration

Tartarus Olympus (the control plane API) supports pluggable persistence backends. For production deployments, **Redis** is required to ensure state durability and high availability.

## Production Configuration

When running in production mode (`TARTARUS_ENV=production`), Olympus enforces the presence of Redis configuration. If Redis is not configured, the service will fail to start.

### Required Environment Variables

| Variable | Description | Required in Production | Default |
|----------|-------------|------------------------|---------|
| `TARTARUS_ENV` | Environment mode (`development` or `production`) | No | `development` |
| `REDIS_ADDR` | Redis server address (e.g., `localhost:6379`) | **Yes** (if env is prod) | - |
| `REDIS_DB` | Redis database number | No | `0` |
| `REDIS_QUEUE_KEY` | Redis key for the work queue | No | `tartarus:queue` |
| `REDIS_PASS` | Redis password (optional) | No | - |

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
