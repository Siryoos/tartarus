# Tartarus Extended Components: Technical Design

## New Chthonic Powers

> *"Beyond the familiar rivers and guardians of the Underworld, other powers dwell in the darkness—forces that govern sleep and death, passage and seasons. These too shall serve Tartarus."*

This document provides detailed technical specifications for the new components introduced in Phases 4-6 of the Tartarus roadmap.

---

## Implementation Status (Reality Check)

The components below are design-only as of Nov 2024. None of their packages exist in the repository yet; current production code relies on the basic Olympus API, Hecatoncheir agent, and related packages.

- **Cerberus**: Not implemented. Current auth is a simple optional bearer key in `pkg/olympus/middleware.go`.
- **Charon**: Not implemented. Requests terminate directly in the Olympus HTTP handlers; no dedicated rate limiter or circuit breaker exists.
- **Hypnos**: Not implemented. There is no sleep/hibernation manager or integration with Nyx snapshots beyond the basic runtime.
- **Thanatos**: Not implemented. Graceful shutdown/termination flows are absent; only ad-hoc kill via Redis pub/sub is wired.
- **Persephone**: Not implemented. No predictive or scheduled scaling logic is present.
- **Phlegethon**: Not implemented. Workload heat classification and pool routing are absent.
- **Typhon**: Not implemented. There is no quarantine pipeline or isolation logic.
- **Kampe**: Not implemented. No legacy container runtime adapters exist.

---

## Table of Contents

1. [Cerberus: The Three-Headed Guardian](#cerberus-the-three-headed-guardian)
2. [Charon: The Ferryman](#charon-the-ferryman)
3. [Hypnos: Lord of Sleep](#hypnos-lord-of-sleep)
4. [Thanatos: The Gentle Death](#thanatos-the-gentle-death)
5. [Persephone: Queen of Seasons](#persephone-queen-of-seasons)
6. [Phlegethon: River of Fire](#phlegethon-river-of-fire)
7. [Typhon: The Chaos Engine](#typhon-the-chaos-engine)
8. [Kampe: The Legacy Bridge](#kampe-the-legacy-bridge)

---

## Cerberus: The Three-Headed Guardian

> *"The three-headed hound Cerberus guards the gates of the Underworld. None may enter or leave without his approval."*

### Purpose

**Status:** Design only; no `pkg/cerberus` exists. API auth today is an optional bearer key in `pkg/olympus/middleware.go`.

Cerberus serves as the unified authentication, authorization, and audit gateway for all Tartarus API access. The three heads represent the three pillars of access control.

### Architecture

```
                         ┌─────────────────────────────────────┐
                         │           CERBERUS                  │
                         │    (Authentication Gateway)         │
                         ├─────────────┬───────────┬───────────┤
                         │   HEAD 1    │  HEAD 2   │  HEAD 3   │
                         │   AuthN     │  AuthZ    │  Audit    │
                         │  (Identity) │ (Permit)  │ (Record)  │
                         └──────┬──────┴─────┬─────┴─────┬─────┘
                                │            │           │
                    ┌───────────┘            │           └───────────┐
                    ▼                        ▼                       ▼
            ┌───────────────┐      ┌───────────────┐      ┌───────────────┐
            │    Identity   │      │  Permission   │      │   Audit Log   │
            │    Providers  │      │    Engine     │      │    Storage    │
            └───────────────┘      └───────────────┘      └───────────────┘
                    │                        │                       │
        ┌───────────┼───────────┐           │                       │
        ▼           ▼           ▼           ▼                       ▼
    ┌───────┐   ┌───────┐   ┌───────┐   ┌───────┐            ┌───────────┐
    │API Key│   │OAuth2 │   │ mTLS  │   │ RBAC  │            │Prometheus │
    └───────┘   └───────┘   └───────┘   │ ABAC  │            │  + Loki   │
                                        └───────┘            └───────────┘
```

### Interface Definition

```go
// pkg/cerberus/gateway.go

package cerberus

import (
    "context"
    "crypto/x509"
    "time"
)

// The three heads of Cerberus
type Gateway interface {
    // Head 1: Authentication - Verify identity
    Authenticate(ctx context.Context, creds Credentials) (*Identity, error)
    
    // Head 2: Authorization - Check permissions
    Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error
    
    // Head 3: Audit - Record all access
    RecordAccess(ctx context.Context, entry *AuditEntry) error
}

// Credentials can be presented in multiple forms
type Credentials interface {
    Type() CredentialType
}

type CredentialType string

const (
    CredentialTypeAPIKey   CredentialType = "api_key"
    CredentialTypeOAuth2   CredentialType = "oauth2"
    CredentialTypeMTLS     CredentialType = "mtls"
    CredentialTypeInternal CredentialType = "internal"
)

// API Key credentials
type APIKeyCredential struct {
    KeyID  string
    Secret string
}

func (c *APIKeyCredential) Type() CredentialType { return CredentialTypeAPIKey }

// OAuth2 token
type OAuth2Credential struct {
    AccessToken string
    TokenType   string
}

func (c *OAuth2Credential) Type() CredentialType { return CredentialTypeOAuth2 }

// mTLS certificate
type MTLSCredential struct {
    Certificate *x509.Certificate
    Chain       []*x509.Certificate
}

func (c *MTLSCredential) Type() CredentialType { return CredentialTypeMTLS }

// Identity represents an authenticated entity
type Identity struct {
    ID           string
    Type         IdentityType
    TenantID     string
    DisplayName  string
    Roles        []string
    Groups       []string
    Attributes   map[string]string
    AuthTime     time.Time
    ExpiresAt    time.Time
}

type IdentityType string

const (
    IdentityTypeUser    IdentityType = "user"
    IdentityTypeService IdentityType = "service"
    IdentityTypeAgent   IdentityType = "agent"
    IdentityTypeSystem  IdentityType = "system"
)

// Action represents an operation being performed
type Action string

const (
    ActionCreate  Action = "create"
    ActionRead    Action = "read"
    ActionUpdate  Action = "update"
    ActionDelete  Action = "delete"
    ActionExecute Action = "execute"
    ActionAdmin   Action = "admin"
)

// Resource represents what is being accessed
type Resource struct {
    Type      ResourceType
    ID        string
    TenantID  string
    Namespace string
}

type ResourceType string

const (
    ResourceTypeSandbox  ResourceType = "sandbox"
    ResourceTypeTemplate ResourceType = "template"
    ResourceTypeSnapshot ResourceType = "snapshot"
    ResourceTypePolicy   ResourceType = "policy"
    ResourceTypeNode     ResourceType = "node"
)

// AuditEntry captures all access for compliance
type AuditEntry struct {
    Timestamp    time.Time
    RequestID    string
    Identity     *Identity
    Action       Action
    Resource     Resource
    Result       AuditResult
    Latency      time.Duration
    SourceIP     string
    UserAgent    string
    ErrorMessage string
}

type AuditResult string

const (
    AuditResultSuccess AuditResult = "success"
    AuditResultDenied  AuditResult = "denied"
    AuditResultError   AuditResult = "error"
)
```

### RBAC Policy Example

```yaml
# policies/cerberus-rbac.yaml

roles:
  - name: sandbox-user
    permissions:
      - resource: sandbox
        actions: [create, read, delete]
        conditions:
          tenant: "{{identity.tenant_id}}"
      - resource: template
        actions: [read]
        
  - name: sandbox-admin
    inherits: [sandbox-user]
    permissions:
      - resource: sandbox
        actions: [create, read, update, delete, admin]
      - resource: template
        actions: [create, read, update, delete]
      - resource: snapshot
        actions: [create, read, delete]
        
  - name: platform-admin
    permissions:
      - resource: "*"
        actions: ["*"]

bindings:
  - identity: "user:alice@example.com"
    roles: [sandbox-admin]
    
  - identity: "group:developers"
    roles: [sandbox-user]
    
  - identity: "service:olympus-api"
    roles: [platform-admin]
```

---

## Charon: The Ferryman

> *"Charon ferries the souls of the dead across the river Styx. For those with proper passage, the crossing is swift; for those without, eternal wandering."*

### Purpose

**Status:** Design only; there is no `pkg/charon` load balancer. Requests currently terminate directly in the Olympus API handlers without dedicated rate limiting or circuit breaking.

Charon provides load balancing, request routing, and traffic management for the Olympus API layer. It ensures requests reach healthy backends and provides circuit breaker protection.

### Architecture

```
                           ┌───────────────────────────────────────┐
                           │              CHARON                   │
                           │         (Request Ferry)               │
                           ├───────────────────────────────────────┤
                           │  ┌─────────────────────────────────┐  │
                           │  │       Rate Limiter              │  │
                           │  │    (Obol collection)            │  │
                           │  └─────────────────────────────────┘  │
                           │                  │                    │
                           │  ┌───────────────▼───────────────┐    │
                           │  │      Circuit Breaker          │    │
                           │  │   (Passage guardian)          │    │
                           │  └───────────────────────────────┘    │
                           │                  │                    │
                           │  ┌───────────────▼───────────────┐    │
                           │  │      Load Balancer            │    │
                           │  │    (Shore selection)          │    │
                           │  └───────────────────────────────┘    │
                           └──────────────────┬────────────────────┘
                                              │
              ┌───────────────────────────────┼───────────────────────────────┐
              │                               │                               │
              ▼                               ▼                               ▼
    ┌───────────────────┐         ┌───────────────────┐         ┌───────────────────┐
    │  Olympus Shore 1  │         │  Olympus Shore 2  │         │  Olympus Shore 3  │
    │   (Primary DC)    │         │  (Secondary DC)   │         │   (Edge Node)     │
    └───────────────────┘         └───────────────────┘         └───────────────────┘
```

### Interface Definition

```go
// pkg/charon/ferry.go

package charon

import (
    "context"
    "net/http"
    "time"
)

// Ferry transports requests across the infrastructure
type Ferry interface {
    // Cross ferries a request to the appropriate backend
    Cross(ctx context.Context, req *http.Request) (*http.Response, error)
    
    // RegisterShore adds a backend destination
    RegisterShore(shore *Shore) error
    
    // DeregisterShore removes a backend
    DeregisterShore(shoreID string) error
    
    // Health returns ferry and shore health status
    Health(ctx context.Context) (*FerryHealth, error)
}

// Shore represents a backend destination (Olympus instance)
type Shore struct {
    ID          string
    Address     string
    Weight      int           // Load balancing weight
    Zone        string        // Geographic zone
    Priority    int           // Failover priority
    HealthCheck *HealthCheck
    Metadata    map[string]string
}

// HealthCheck configuration for shores
type HealthCheck struct {
    Path     string
    Interval time.Duration
    Timeout  time.Duration
    Healthy  int  // Consecutive successes to mark healthy
    Unhealthy int // Consecutive failures to mark unhealthy
}

// FerryHealth reports overall system health
type FerryHealth struct {
    Status     HealthStatus
    Shores     []ShoreHealth
    OpenBreakers int
    QueueDepth   int
}

type ShoreHealth struct {
    ShoreID    string
    Status     HealthStatus
    Latency    time.Duration
    ActiveConns int
    ErrorRate   float64
}

type HealthStatus string

const (
    HealthStatusHealthy   HealthStatus = "healthy"
    HealthStatusDegraded  HealthStatus = "degraded"
    HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// FerryConfig configures the ferry behavior
type FerryConfig struct {
    // Load balancing strategy
    Strategy LoadBalanceStrategy
    
    // Circuit breaker settings
    CircuitBreaker CircuitBreakerConfig
    
    // Rate limiting per tenant
    RateLimiting RateLimitConfig
    
    // Retry configuration
    Retry RetryConfig
    
    // Timeout for crossing
    CrossingTimeout time.Duration
}

type LoadBalanceStrategy string

const (
    StrategyRoundRobin     LoadBalanceStrategy = "round_robin"
    StrategyLeastConn      LoadBalanceStrategy = "least_conn"
    StrategyWeighted       LoadBalanceStrategy = "weighted"
    StrategyIPHash         LoadBalanceStrategy = "ip_hash"
    StrategyZoneAware      LoadBalanceStrategy = "zone_aware"
)

type CircuitBreakerConfig struct {
    Enabled          bool
    Threshold        int           // Failures before opening
    Timeout          time.Duration // Time before half-open
    HalfOpenRequests int           // Requests to test in half-open
}

type RateLimitConfig struct {
    Enabled     bool
    RequestsPerSecond int
    Burst            int
    KeyFunc          string  // "tenant", "ip", "identity"
}

type RetryConfig struct {
    MaxRetries    int
    InitialDelay  time.Duration
    MaxDelay      time.Duration
    RetryOn       []int  // HTTP status codes to retry
}
```

### Implementation Example

```go
// pkg/charon/load_balancer.go

package charon

import (
    "context"
    "math/rand"
    "net/http"
    "sync"
    "sync/atomic"
    "time"
)

type BoatFerry struct {
    shores       []*Shore
    shoreHealth  map[string]*ShoreHealth
    breakers     map[string]*CircuitBreaker
    limiter      *RateLimiter
    config       *FerryConfig
    
    // Round-robin counter
    rrCounter    uint64
    
    mu           sync.RWMutex
}

func NewBoatFerry(config *FerryConfig) *BoatFerry {
    return &BoatFerry{
        shores:      make([]*Shore, 0),
        shoreHealth: make(map[string]*ShoreHealth),
        breakers:    make(map[string]*CircuitBreaker),
        config:      config,
    }
}

func (f *BoatFerry) Cross(ctx context.Context, req *http.Request) (*http.Response, error) {
    // Check rate limit (collecting the obol - payment for passage)
    if f.limiter != nil {
        if err := f.limiter.Allow(ctx, extractTenant(req)); err != nil {
            return nil, &CrossingError{
                Code:    http.StatusTooManyRequests,
                Message: "Rate limit exceeded - the ferryman demands patience",
            }
        }
    }
    
    // Select shore based on strategy
    shore, err := f.selectShore(ctx, req)
    if err != nil {
        return nil, &CrossingError{
            Code:    http.StatusServiceUnavailable,
            Message: "No shores available - the river cannot be crossed",
        }
    }
    
    // Check circuit breaker
    breaker := f.breakers[shore.ID]
    if breaker != nil && !breaker.Allow() {
        // Try another shore
        return f.retryWithFallback(ctx, req, shore.ID)
    }
    
    // Forward request
    resp, err := f.forwardRequest(ctx, req, shore)
    if err != nil {
        if breaker != nil {
            breaker.RecordFailure()
        }
        // Retry if configured
        if f.config.Retry.MaxRetries > 0 {
            return f.retryRequest(ctx, req, shore.ID, 0)
        }
        return nil, err
    }
    
    if breaker != nil {
        breaker.RecordSuccess()
    }
    
    return resp, nil
}

func (f *BoatFerry) selectShore(ctx context.Context, req *http.Request) (*Shore, error) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    
    // Filter healthy shores
    healthy := make([]*Shore, 0)
    for _, shore := range f.shores {
        if h, ok := f.shoreHealth[shore.ID]; ok && h.Status == HealthStatusHealthy {
            healthy = append(healthy, shore)
        }
    }
    
    if len(healthy) == 0 {
        return nil, ErrNoHealthyShores
    }
    
    switch f.config.Strategy {
    case StrategyRoundRobin:
        idx := atomic.AddUint64(&f.rrCounter, 1) % uint64(len(healthy))
        return healthy[idx], nil
        
    case StrategyLeastConn:
        var selected *Shore
        minConns := int(^uint(0) >> 1) // Max int
        for _, shore := range healthy {
            if h := f.shoreHealth[shore.ID]; h.ActiveConns < minConns {
                minConns = h.ActiveConns
                selected = shore
            }
        }
        return selected, nil
        
    case StrategyWeighted:
        totalWeight := 0
        for _, shore := range healthy {
            totalWeight += shore.Weight
        }
        r := rand.Intn(totalWeight)
        for _, shore := range healthy {
            r -= shore.Weight
            if r < 0 {
                return shore, nil
            }
        }
        return healthy[0], nil
        
    default:
        return healthy[rand.Intn(len(healthy))], nil
    }
}
```

---

## Hypnos: Lord of Sleep

> *"Hypnos, god of sleep, dwells in a cave where the sun never shines. His gentle touch brings rest to mortals and gods alike."*

### Purpose

**Status:** Design only; no `pkg/hypnos` exists and the current runtime has no sleep/hibernation path.

Hypnos manages VM hibernation for cost optimization and resource efficiency. When sandboxes are idle, Hypnos puts them to sleep; when needed, they wake refreshed.

### Architecture

```
                         ┌─────────────────────────────────────┐
                         │             HYPNOS                  │
                         │      (Sleep Manager)                │
                         ├─────────────────────────────────────┤
                         │                                     │
                         │   ┌───────────────────────────┐     │
                         │   │    Idle Detection         │     │
                         │   │  (The Watchful Drowsiness)│     │
                         │   └─────────────┬─────────────┘     │
                         │                 │                   │
                         │   ┌─────────────▼─────────────┐     │
                         │   │   Memory Compression      │     │
                         │   │   (Dreams Compressed)     │     │
                         │   └─────────────┬─────────────┘     │
                         │                 │                   │
                         │   ┌─────────────▼─────────────┐     │
                         │   │   Sleep Store (Erebus)    │     │
                         │   │   (The Cave of Sleep)     │     │
                         │   └───────────────────────────┘     │
                         │                                     │
                         └─────────────────────────────────────┘
                                           │
                    ┌──────────────────────┼──────────────────────┐
                    │                      │                      │
                    ▼                      ▼                      ▼
            ┌───────────────┐      ┌───────────────┐      ┌───────────────┐
            │   Sleeping    │      │   Sleeping    │      │   Sleeping    │
            │   VM Alpha    │      │   VM Beta     │      │   VM Gamma    │
            │   (zzzZZZ)    │      │   (zzzZZZ)    │      │   (zzzZZZ)    │
            └───────────────┘      └───────────────┘      └───────────────┘
```

### Interface Definition

```go
// pkg/hypnos/sleep.go

package hypnos

import (
    "context"
    "time"
    
    "github.com/yourorg/tartarus/pkg/erebus"
    "github.com/yourorg/tartarus/pkg/nyx"
    "github.com/yourorg/tartarus/pkg/tartarus"
)

// SleepManager handles VM hibernation and wake-up
type SleepManager interface {
    // Sleep puts a running sandbox into hibernation
    Sleep(ctx context.Context, sandboxID string, opts *SleepOptions) (*SleepRecord, error)
    
    // Wake restores a sleeping sandbox
    Wake(ctx context.Context, sandboxID string) (*WakeResult, error)
    
    // IsSleeping checks if a sandbox is hibernating
    IsSleeping(sandboxID string) bool
    
    // ScheduleSleep sets up automatic sleep after idle period
    ScheduleSleep(sandboxID string, idleThreshold time.Duration) error
    
    // CancelScheduledSleep cancels pending sleep
    CancelScheduledSleep(sandboxID string) error
    
    // ListSleeping returns all hibernating sandboxes
    ListSleeping(ctx context.Context, filter *SleepFilter) ([]*SleepRecord, error)
}

type SleepOptions struct {
    // Compress memory before storing (slower but smaller)
    CompressMemory bool
    
    // Maximum time to keep sleeping (auto-terminate after)
    MaxSleepDuration time.Duration
    
    // Priority for wake-up (higher = faster restoration)
    WakePriority int
    
    // Preserve network state (TAP device, IP)
    PreserveNetwork bool
}

type SleepRecord struct {
    SandboxID       string
    SleepTime       time.Time
    ScheduledWake   *time.Time
    MemorySize      int64
    CompressedSize  int64
    SnapshotPath    string
    WakePriority    int
    TenantID        string
}

type WakeResult struct {
    SandboxID    string
    WakeTime     time.Time
    SleepDuration time.Duration
    RestoreTime   time.Duration
    Status       SandboxStatus
}

type SleepFilter struct {
    TenantID        string
    SleepingBefore  time.Time
    SleepingAfter   time.Time
    MinMemorySize   int64
    MaxMemorySize   int64
}

// HypnosManager implements SleepManager
type HypnosManager struct {
    runtime     tartarus.Runtime
    snapshotMgr nyx.Manager
    sleepStore  erebus.Store
    
    // Track sleeping sandboxes
    sleeping    map[string]*SleepRecord
    
    // Scheduled sleep timers
    schedules   map[string]*time.Timer
    
    // Wake triggers for fast restoration
    triggers    map[string]chan struct{}
    
    // Configuration
    config      *HypnosConfig
}

type HypnosConfig struct {
    // Default idle threshold for auto-sleep
    DefaultIdleThreshold time.Duration
    
    // Memory compression algorithm
    CompressionAlgorithm string  // "zstd", "lz4", "none"
    
    // Maximum concurrent wake operations
    MaxConcurrentWakes int
    
    // Pre-warm pool size (keep some slots ready for fast wake)
    PrewarmPoolSize int
}
```

### Implementation Details

```go
// pkg/hypnos/manager.go

package hypnos

import (
    "compress/zlib"
    "context"
    "fmt"
    "io"
    "sync"
    "time"
)

func NewHypnosManager(
    runtime tartarus.Runtime,
    snapshotMgr nyx.Manager,
    sleepStore erebus.Store,
    config *HypnosConfig,
) *HypnosManager {
    return &HypnosManager{
        runtime:     runtime,
        snapshotMgr: snapshotMgr,
        sleepStore:  sleepStore,
        sleeping:    make(map[string]*SleepRecord),
        schedules:   make(map[string]*time.Timer),
        triggers:    make(map[string]chan struct{}),
        config:      config,
    }
}

func (m *HypnosManager) Sleep(ctx context.Context, sandboxID string, opts *SleepOptions) (*SleepRecord, error) {
    // 1. Pause the VM
    if err := m.runtime.Pause(ctx, sandboxID); err != nil {
        return nil, fmt.Errorf("failed to pause sandbox: %w", err)
    }
    
    // 2. Create memory snapshot
    snapshot, err := m.snapshotMgr.CreateFromRunning(ctx, sandboxID)
    if err != nil {
        // Resume on failure
        _ = m.runtime.Resume(ctx, sandboxID)
        return nil, fmt.Errorf("failed to create snapshot: %w", err)
    }
    
    // 3. Compress if requested
    var finalSize int64 = snapshot.MemorySize
    var snapshotPath string = snapshot.Path
    
    if opts.CompressMemory {
        compressedPath, compressedSize, err := m.compressSnapshot(ctx, snapshot.Path)
        if err != nil {
            // Continue with uncompressed
            m.logWarn("compression failed, using uncompressed snapshot", err)
        } else {
            finalSize = compressedSize
            snapshotPath = compressedPath
        }
    }
    
    // 4. Store snapshot in sleep store (the cave of Hypnos)
    sleepPath := fmt.Sprintf("sleep/%s/%s", sandboxID, time.Now().Format(time.RFC3339))
    if err := m.sleepStore.Put(sleepPath, snapshotPath); err != nil {
        return nil, fmt.Errorf("failed to store sleep state: %w", err)
    }
    
    // 5. Release VM resources
    if err := m.runtime.Destroy(ctx, sandboxID); err != nil {
        m.logWarn("failed to destroy paused VM", err)
    }
    
    // 6. Create sleep record
    record := &SleepRecord{
        SandboxID:      sandboxID,
        SleepTime:      time.Now(),
        MemorySize:     snapshot.MemorySize,
        CompressedSize: finalSize,
        SnapshotPath:   sleepPath,
        WakePriority:   opts.WakePriority,
    }
    
    m.mu.Lock()
    m.sleeping[sandboxID] = record
    m.triggers[sandboxID] = make(chan struct{})
    m.mu.Unlock()
    
    return record, nil
}

func (m *HypnosManager) Wake(ctx context.Context, sandboxID string) (*WakeResult, error) {
    m.mu.RLock()
    record, exists := m.sleeping[sandboxID]
    m.mu.RUnlock()
    
    if !exists {
        return nil, fmt.Errorf("sandbox %s is not sleeping", sandboxID)
    }
    
    wakeStart := time.Now()
    
    // 1. Retrieve snapshot from sleep store
    snapshotReader, err := m.sleepStore.Get(record.SnapshotPath)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve sleep state: %w", err)
    }
    defer snapshotReader.Close()
    
    // 2. Decompress if needed
    var memoryReader io.Reader = snapshotReader
    if record.CompressedSize < record.MemorySize {
        memoryReader, err = zlib.NewReader(snapshotReader)
        if err != nil {
            return nil, fmt.Errorf("failed to decompress: %w", err)
        }
    }
    
    // 3. Restore VM from snapshot
    if err := m.snapshotMgr.RestoreToSandbox(ctx, sandboxID, memoryReader); err != nil {
        return nil, fmt.Errorf("failed to restore sandbox: %w", err)
    }
    
    // 4. Resume execution
    if err := m.runtime.Resume(ctx, sandboxID); err != nil {
        return nil, fmt.Errorf("failed to resume sandbox: %w", err)
    }
    
    // 5. Cleanup
    m.mu.Lock()
    delete(m.sleeping, sandboxID)
    close(m.triggers[sandboxID])
    delete(m.triggers, sandboxID)
    m.mu.Unlock()
    
    return &WakeResult{
        SandboxID:     sandboxID,
        WakeTime:      time.Now(),
        SleepDuration: time.Since(record.SleepTime),
        RestoreTime:   time.Since(wakeStart),
        Status:        SandboxStatusRunning,
    }, nil
}

// Idle detection runs in background, watching for sandboxes to put to sleep
func (m *HypnosManager) runIdleWatcher(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.checkIdleSandboxes(ctx)
        }
    }
}

func (m *HypnosManager) checkIdleSandboxes(ctx context.Context) {
    sandboxes, err := m.runtime.List(ctx)
    if err != nil {
        return
    }
    
    for _, sb := range sandboxes {
        if sb.IdleSince != nil && time.Since(*sb.IdleSince) > m.config.DefaultIdleThreshold {
            // This sandbox has been idle too long - time for sleep
            go func(id string) {
                _, err := m.Sleep(ctx, id, &SleepOptions{
                    CompressMemory: true,
                })
                if err != nil {
                    m.logWarn("failed to auto-sleep idle sandbox", err)
                }
            }(sb.ID)
        }
    }
}
```

---

## Thanatos: The Gentle Death

> *"Thanatos, twin of Hypnos, brings peaceful death. Unlike his violent brothers, his touch is gentle—he guides souls to their final rest."*

### Purpose

**Status:** Design only; there is no `pkg/thanatos` and kill/shutdown flows are limited to ad-hoc Redis pub/sub signals.

Thanatos handles graceful termination of sandboxes, ensuring clean shutdown, state preservation when needed, and proper resource cleanup.

### Interface Definition

```go
// pkg/thanatos/termination.go

package thanatos

import (
    "context"
    "time"
)

// TerminationHandler manages graceful sandbox deaths
type TerminationHandler interface {
    // Initiate graceful shutdown with grace period
    InitiateShutdown(ctx context.Context, sandboxID string, opts *ShutdownOptions) error
    
    // Create checkpoint before termination (for potential resurrection)
    Checkpoint(ctx context.Context, sandboxID string) (*Checkpoint, error)
    
    // Force immediate termination (when grace period exceeded)
    ForceTerminate(ctx context.Context, sandboxID string) error
    
    // Get termination status
    Status(ctx context.Context, sandboxID string) (*TerminationStatus, error)
    
    // List pending terminations
    ListPending(ctx context.Context) ([]*TerminationStatus, error)
}

type ShutdownOptions struct {
    // Grace period for clean shutdown
    GracePeriod time.Duration
    
    // Reason for termination
    Reason TerminationReason
    
    // Whether to create a checkpoint before death
    CreateCheckpoint bool
    
    // Signal to send (SIGTERM, SIGINT, SIGKILL)
    Signal Signal
    
    // Callback URL for completion notification
    CallbackURL string
}

type TerminationReason string

const (
    ReasonCompleted       TerminationReason = "completed"
    ReasonTimeout         TerminationReason = "timeout"
    ReasonUserRequest     TerminationReason = "user_request"
    ReasonResourceLimit   TerminationReason = "resource_limit"
    ReasonPolicyViolation TerminationReason = "policy_violation"
    ReasonNodeDrain       TerminationReason = "node_drain"
    ReasonSystemShutdown  TerminationReason = "system_shutdown"
)

type Signal string

const (
    SignalTerm Signal = "SIGTERM"
    SignalInt  Signal = "SIGINT"
    SignalKill Signal = "SIGKILL"
    SignalHup  Signal = "SIGHUP"
)

type TerminationStatus struct {
    SandboxID        string
    Phase            TerminationPhase
    Reason           TerminationReason
    InitiatedAt      time.Time
    GraceDeadline    time.Time
    CheckpointPath   string
    ExitCode         *int
    ErrorMessage     string
}

type TerminationPhase string

const (
    PhaseInitiated    TerminationPhase = "initiated"
    PhaseSignaled     TerminationPhase = "signaled"
    PhaseDraining     TerminationPhase = "draining"
    PhaseCheckpointing TerminationPhase = "checkpointing"
    PhaseTerminating  TerminationPhase = "terminating"
    PhaseCompleted    TerminationPhase = "completed"
    PhaseFailed       TerminationPhase = "failed"
)

type Checkpoint struct {
    ID          string
    SandboxID   string
    CreatedAt   time.Time
    Size        int64
    Path        string
    Restorable  bool
    ExpiresAt   time.Time
}

// ThanatosHandler implements TerminationHandler
type ThanatosHandler struct {
    runtime     tartarus.Runtime
    snapshotMgr nyx.Manager
    store       erebus.Store
    pending     map[string]*TerminationStatus
    
    // Pre-mortem hooks (run before termination)
    preMortemHooks []PreMortemHook
    
    // Post-mortem hooks (run after termination)
    postMortemHooks []PostMortemHook
}

// PreMortemHook runs before sandbox death
type PreMortemHook func(ctx context.Context, sandboxID string, reason TerminationReason) error

// PostMortemHook runs after sandbox death
type PostMortemHook func(ctx context.Context, sandboxID string, exitCode int, duration time.Duration) error
```

### Graceful Shutdown Flow

```
┌─────────────────┐
│ Termination     │
│ Request         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│ Phase: Initiated│────▶│ Pre-Mortem      │
│                 │     │ Hooks           │
└────────┬────────┘     └─────────────────┘
         │
         ▼
┌─────────────────┐
│ Phase: Signaled │  Send SIGTERM
│                 │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Phase: Draining │  Wait for connections
│                 │  to close
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼ (if checkpoint requested)
┌───────┐  ┌─────────────────┐
│       │  │ Phase:          │
│       │  │ Checkpointing   │
│       │  └────────┬────────┘
│       │           │
│       │◀──────────┘
│       │
    ▼
┌─────────────────┐
│ Phase:          │  Release resources
│ Terminating     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│ Phase: Completed│────▶│ Post-Mortem     │
│                 │     │ Hooks           │
└─────────────────┘     └─────────────────┘
```

---

## Persephone: Queen of Seasons

> *"Persephone spends half the year in the Underworld with Hades, and half above with her mother Demeter. Her comings and goings mark the seasons of the world."*

### Purpose

**Status:** Design only; no seasonal or predictive scaling exists in the codebase.

Persephone manages seasonal scaling patterns, predictive autoscaling, and capacity planning based on historical usage patterns.

### Interface Definition

```go
// pkg/persephone/seasons.go

package persephone

import (
    "context"
    "time"
)

// SeasonalScaler manages predictive and time-based scaling
type SeasonalScaler interface {
    // Forecast predicts future demand
    Forecast(ctx context.Context, window time.Duration) (*Forecast, error)
    
    // DefineSeason creates a seasonal scaling rule
    DefineSeason(ctx context.Context, season *Season) error
    
    // ApplySeason activates a seasonal configuration
    ApplySeason(ctx context.Context, seasonID string) error
    
    // Learn updates the model with historical data
    Learn(ctx context.Context, history []*UsageRecord) error
    
    // CurrentSeason returns the active season
    CurrentSeason(ctx context.Context) (*Season, error)
    
    // RecommendCapacity suggests optimal resource levels
    RecommendCapacity(ctx context.Context, targetUtil float64) (*CapacityRecommendation, error)
}

// Season defines a time-based scaling configuration
type Season struct {
    ID          string
    Name        string
    Description string
    
    // When this season applies
    Schedule    SeasonSchedule
    
    // Scaling parameters
    MinNodes    int
    MaxNodes    int
    TargetUtilization float64
    
    // Pre-warming configuration
    Prewarming  PrewarmConfig
    
    // Resource class distribution
    ResourceMix map[string]float64
}

type SeasonSchedule struct {
    // Cron-style schedules
    StartCron   string  // e.g., "0 8 * * MON-FRI" (8am weekdays)
    EndCron     string  // e.g., "0 18 * * MON-FRI" (6pm weekdays)
    
    // Or specific time ranges
    TimeRanges  []TimeRange
    
    // Timezone
    Timezone    string
}

type TimeRange struct {
    Start time.Time
    End   time.Time
}

type PrewarmConfig struct {
    // Templates to pre-warm
    Templates   []string
    
    // Number of warm instances per template
    PoolSize    int
    
    // How far ahead to start pre-warming
    LeadTime    time.Duration
}

type Forecast struct {
    GeneratedAt time.Time
    Window      time.Duration
    Predictions []Prediction
    Confidence  float64
}

type Prediction struct {
    Time             time.Time
    PredictedDemand  int
    UpperBound       int
    LowerBound       int
    Confidence       float64
}

type UsageRecord struct {
    Timestamp    time.Time
    ActiveVMs    int
    QueueDepth   int
    CPUUtil      float64
    MemoryUtil   float64
    LaunchCount  int
    ErrorCount   int
}

type CapacityRecommendation struct {
    CurrentNodes    int
    RecommendedNodes int
    Reason          string
    CostDelta       float64
    ConfidenceLevel float64
}

// Example seasons (Greek mythology inspired)
var (
    SeasonSpring = &Season{
        ID:          "spring",
        Name:        "Spring Growth",
        Description: "Gradual scaling up as demand increases",
        Schedule: SeasonSchedule{
            StartCron: "0 6 * * *",
            EndCron:   "0 18 * * *",
        },
        MinNodes:          5,
        MaxNodes:          50,
        TargetUtilization: 0.7,
        Prewarming: PrewarmConfig{
            Templates: []string{"python-ds", "node-18"},
            PoolSize:  10,
            LeadTime:  30 * time.Minute,
        },
    }
    
    SeasonSummer = &Season{
        ID:          "summer",
        Name:        "Peak Summer",
        Description: "Maximum capacity for peak demand",
        MinNodes:          20,
        MaxNodes:          100,
        TargetUtilization: 0.8,
    }
    
    SeasonAutumn = &Season{
        ID:          "autumn",
        Name:        "Autumn Harvest",
        Description: "Gradual wind-down from peak",
        MinNodes:          10,
        MaxNodes:          60,
        TargetUtilization: 0.7,
    }
    
    SeasonWinter = &Season{
        ID:          "winter",
        Name:        "Winter Rest",
        Description: "Minimal capacity during low demand",
        MinNodes:          3,
        MaxNodes:          20,
        TargetUtilization: 0.5,
    }
)
```

---

## Phlegethon: River of Fire

> *"The Phlegethon, river of fire, coils around the earth and flows into Tartarus. Its flames burn but do not consume—eternal punishment for the wicked."*

### Purpose

**Status:** Design only; workload heat classification and pool routing are not implemented.

Phlegethon routes high-compute, "hot" workloads to appropriate resources. It classifies workload intensity and ensures heavy tasks don't starve lighter ones.

### Interface Definition

```go
// pkg/phlegethon/router.go

package phlegethon

import (
    "context"
    "time"
)

// HeatRouter classifies and routes workloads by intensity
type HeatRouter interface {
    // ClassifyHeat determines workload intensity
    ClassifyHeat(ctx context.Context, req *SandboxRequest) (HeatLevel, error)
    
    // RouteToPool selects appropriate resource pool
    RouteToPool(ctx context.Context, heat HeatLevel) (*PoolAssignment, error)
    
    // GetResourceClass returns the resource class for a heat level
    GetResourceClass(heat HeatLevel) *ResourceClass
    
    // DefinePool creates a new resource pool
    DefinePool(ctx context.Context, pool *ResourcePool) error
    
    // GetHeatMetrics returns current heat distribution
    GetHeatMetrics(ctx context.Context) (*HeatMetrics, error)
}

// HeatLevel represents workload intensity
type HeatLevel string

const (
    // Cold: Quick, lightweight tasks (< 10s, < 1 CPU, < 512MB)
    HeatCold HeatLevel = "cold"
    
    // Warm: Standard workloads (< 5min, < 4 CPU, < 4GB)
    HeatWarm HeatLevel = "warm"
    
    // Hot: CPU-intensive workloads (< 30min, < 8 CPU, < 16GB)
    HeatHot HeatLevel = "hot"
    
    // Inferno: Long-running, high-resource (unlimited duration, GPU possible)
    HeatInferno HeatLevel = "inferno"
)

// ResourceClass defines resource allocation for a heat level
type ResourceClass struct {
    Name         string
    Heat         HeatLevel
    CPUCores     int
    MemoryMB     int
    GPUCount     int
    MaxDuration  time.Duration
    NetworkBurst int  // Mbps
    DiskIOPS     int
}

var DefaultResourceClasses = map[HeatLevel]*ResourceClass{
    HeatCold: {
        Name:        "ember",
        Heat:        HeatCold,
        CPUCores:    1,
        MemoryMB:    512,
        MaxDuration: 30 * time.Second,
    },
    HeatWarm: {
        Name:        "flame",
        Heat:        HeatWarm,
        CPUCores:    2,
        MemoryMB:    2048,
        MaxDuration: 5 * time.Minute,
    },
    HeatHot: {
        Name:        "blaze",
        Heat:        HeatHot,
        CPUCores:    4,
        MemoryMB:    8192,
        MaxDuration: 30 * time.Minute,
    },
    HeatInferno: {
        Name:        "inferno",
        Heat:        HeatInferno,
        CPUCores:    8,
        MemoryMB:    32768,
        GPUCount:    1,
        MaxDuration: 24 * time.Hour,
    },
}

// ResourcePool is a set of nodes dedicated to a heat level
type ResourcePool struct {
    ID           string
    Name         string
    Heat         HeatLevel
    NodeSelector map[string]string  // Label selector
    MinNodes     int
    MaxNodes     int
    CurrentNodes int
}

type PoolAssignment struct {
    Pool         *ResourcePool
    Node         string
    ResourceClass *ResourceClass
}

type HeatMetrics struct {
    TotalSandboxes  int
    ByHeat          map[HeatLevel]int
    QueueDepthByHeat map[HeatLevel]int
    AvgWaitByHeat   map[HeatLevel]time.Duration
}

// HeatClassifier uses heuristics to classify workloads
type HeatClassifier struct {
    // Historical data for learning
    history map[string][]HeatObservation
    
    // Template-based hints
    templateHints map[string]HeatLevel
}

type HeatObservation struct {
    TemplateID    string
    ActualDuration time.Duration
    PeakCPU       float64
    PeakMemory    int64
    Timestamp     time.Time
}

func (c *HeatClassifier) Classify(req *SandboxRequest) HeatLevel {
    // 1. Check explicit hint
    if req.HeatHint != "" {
        return req.HeatHint
    }
    
    // 2. Check template-based hint
    if hint, ok := c.templateHints[req.TemplateID]; ok {
        return hint
    }
    
    // 3. Use resource request as indicator
    if req.MaxDuration > 10*time.Minute || req.CPUCores >= 4 {
        return HeatInferno
    }
    if req.MaxDuration > 2*time.Minute || req.CPUCores >= 2 {
        return HeatHot
    }
    if req.MaxDuration > 30*time.Second {
        return HeatWarm
    }
    
    return HeatCold
}
```

---

## Typhon: The Chaos Engine

> *"Typhon, father of monsters, was cast into Tartarus by Zeus's thunderbolts. There he rages still, source of volcanic fires and storms. Bound, but never truly defeated."*

### Purpose

**Status:** Design only; no quarantine pipeline exists and risky workloads are treated the same as normal ones.

Typhon manages the quarantine pool for high-risk, untrusted workloads that require maximum isolation and monitoring.

### Interface Definition

```go
// pkg/typhon/quarantine.go

package typhon

import (
    "context"
    "time"
)

// QuarantineManager handles high-risk workload isolation
type QuarantineManager interface {
    // Quarantine moves a sandbox to the quarantine pool
    Quarantine(ctx context.Context, req *QuarantineRequest) (*QuarantineRecord, error)
    
    // Release removes a sandbox from quarantine
    Release(ctx context.Context, sandboxID string, approval *ReleaseApproval) error
    
    // Examine analyzes a quarantined sandbox
    Examine(ctx context.Context, sandboxID string) (*ExaminationReport, error)
    
    // ListQuarantined returns all sandboxes in quarantine
    ListQuarantined(ctx context.Context, filter *QuarantineFilter) ([]*QuarantineRecord, error)
    
    // SetPolicy configures quarantine policies
    SetPolicy(ctx context.Context, policy *QuarantinePolicy) error
}

type QuarantineRequest struct {
    SandboxID     string
    Reason        QuarantineReason
    Evidence      []Evidence
    RequestedBy   string
    AutoQuarantine bool
}

type QuarantineReason string

const (
    ReasonSuspiciousBehavior   QuarantineReason = "suspicious_behavior"
    ReasonPolicyViolation      QuarantineReason = "policy_violation"
    ReasonNetworkAnomaly       QuarantineReason = "network_anomaly"
    ReasonResourceAbuse        QuarantineReason = "resource_abuse"
    ReasonUntrustedSource      QuarantineReason = "untrusted_source"
    ReasonManualFlag           QuarantineReason = "manual_flag"
    ReasonSecurityScan         QuarantineReason = "security_scan"
)

type Evidence struct {
    Type        EvidenceType
    Description string
    Data        []byte
    Timestamp   time.Time
}

type EvidenceType string

const (
    EvidenceTypeNetworkLog    EvidenceType = "network_log"
    EvidenceTypeSyscallTrace  EvidenceType = "syscall_trace"
    EvidenceTypeFileAccess    EvidenceType = "file_access"
    EvidenceTypeResourceSpike EvidenceType = "resource_spike"
    EvidenceTypeScreenshot    EvidenceType = "screenshot"
)

type QuarantineRecord struct {
    SandboxID       string
    Reason          QuarantineReason
    QuarantinedAt   time.Time
    QuarantinedBy   string
    Evidence        []Evidence
    Node            string          // Dedicated quarantine node
    Status          QuarantineStatus
    ExaminationCount int
}

type QuarantineStatus string

const (
    StatusActive    QuarantineStatus = "active"
    StatusExamining QuarantineStatus = "examining"
    StatusReleased  QuarantineStatus = "released"
    StatusDestroyed QuarantineStatus = "destroyed"
)

type ExaminationReport struct {
    SandboxID     string
    ExaminedAt    time.Time
    Findings      []Finding
    RiskScore     float64  // 0.0 (safe) to 1.0 (dangerous)
    Recommendation RecommendedAction
}

type Finding struct {
    Severity    Severity
    Category    string
    Description string
    Evidence    *Evidence
}

type Severity string

const (
    SeverityInfo     Severity = "info"
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

type RecommendedAction string

const (
    ActionRelease    RecommendedAction = "release"
    ActionMonitor    RecommendedAction = "monitor"
    ActionDestroy    RecommendedAction = "destroy"
    ActionEscalate   RecommendedAction = "escalate"
)

type ReleaseApproval struct {
    ApprovedBy   string
    Reason       string
    Conditions   []string  // Conditions for release
    ExpiresAt    *time.Time
}

type QuarantinePolicy struct {
    // Automatic quarantine triggers
    AutoTriggers []AutoQuarantineTrigger
    
    // Dedicated quarantine nodes
    QuarantineNodes []string
    
    // Enhanced isolation settings
    Isolation QuarantineIsolation
    
    // Retention period
    MaxQuarantineDuration time.Duration
}

type AutoQuarantineTrigger struct {
    Condition   string  // CEL expression
    Reason      QuarantineReason
    Enabled     bool
}

type QuarantineIsolation struct {
    // Network isolation
    NetworkMode     NetworkMode
    AllowedEgress   []string  // Only specific endpoints
    
    // Enhanced seccomp profile
    SeccompProfile  string
    
    // Separate storage backend
    StorageBackend  string
    
    // Additional monitoring
    EnableStrace    bool
    EnableAuditd    bool
    RecordNetwork   bool
}

type NetworkMode string

const (
    NetworkModeNone       NetworkMode = "none"
    NetworkModeRestricted NetworkMode = "restricted"
    NetworkModeMonitored  NetworkMode = "monitored"
)
```

---

## Kampe: The Legacy Bridge

> *"Kampe was the jailer set by Kronos to guard the Cyclopes and Hecatoncheires in Tartarus. Zeus slew her to free them for his war. She represents the old order that must give way to the new."*

### Purpose

**Status:** Design only; there are no legacy container runtime adapters or migration helpers.

Kampe provides compatibility shims for legacy container runtimes (Docker, containerd), allowing gradual migration from containers to microVMs.

### Interface Definition

```go
// pkg/kampe/legacy.go

package kampe

import (
    "context"
    "io"
    
    "github.com/yourorg/tartarus/pkg/tartarus"
)

// LegacyRuntime wraps legacy container runtimes with the Tartarus interface
type LegacyRuntime interface {
    tartarus.Runtime
    
    // Migration helpers
    CanMigrate(ctx context.Context, containerID string) (bool, error)
    MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error)
    ExportState(ctx context.Context, containerID string) (*ContainerState, error)
}

// MigrationPlan describes steps to move from container to microVM
type MigrationPlan struct {
    ContainerID      string
    TargetTemplate   string
    RequiredChanges  []MigrationChange
    EstimatedDowntime time.Duration
    RiskLevel        RiskLevel
    Recommendations  []string
}

type MigrationChange struct {
    Type        ChangeType
    Description string
    Required    bool
    AutoFix     bool
}

type ChangeType string

const (
    ChangeTypeFilesystem ChangeType = "filesystem"
    ChangeTypeNetwork    ChangeType = "network"
    ChangeTypeResources  ChangeType = "resources"
    ChangeTypeEnvironment ChangeType = "environment"
    ChangeTypeEntrypoint ChangeType = "entrypoint"
)

type RiskLevel string

const (
    RiskLevelLow    RiskLevel = "low"
    RiskLevelMedium RiskLevel = "medium"
    RiskLevelHigh   RiskLevel = "high"
)

type ContainerState struct {
    ID          string
    Image       string
    Config      ContainerConfig
    Filesystem  []FileEntry
    Environment map[string]string
    Processes   []ProcessInfo
}

type ContainerConfig struct {
    Entrypoint []string
    Cmd        []string
    WorkingDir string
    User       string
    Env        []string
    Volumes    []string
    Ports      []PortMapping
}

// DockerAdapter wraps Docker Engine
type DockerAdapter struct {
    client     *docker.Client
    converter  *ImageConverter
}

func NewDockerAdapter(socketPath string) (*DockerAdapter, error) {
    client, err := docker.NewClientWithOpts(
        docker.WithHost("unix://" + socketPath),
        docker.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, err
    }
    return &DockerAdapter{
        client:    client,
        converter: NewImageConverter(),
    }, nil
}

func (d *DockerAdapter) Create(ctx context.Context, cfg *tartarus.VMConfig) (string, error) {
    // Convert Tartarus config to Docker container config
    dockerCfg := d.convertConfig(cfg)
    
    resp, err := d.client.ContainerCreate(ctx, dockerCfg.Config, dockerCfg.HostConfig, nil, nil, "")
    if err != nil {
        return "", err
    }
    
    return resp.ID, nil
}

func (d *DockerAdapter) MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error) {
    // 1. Inspect container
    info, err := d.client.ContainerInspect(ctx, containerID)
    if err != nil {
        return nil, err
    }
    
    plan := &MigrationPlan{
        ContainerID: containerID,
    }
    
    // 2. Analyze for compatibility issues
    
    // Check for host mounts
    for _, mount := range info.Mounts {
        if mount.Type == "bind" {
            plan.RequiredChanges = append(plan.RequiredChanges, MigrationChange{
                Type:        ChangeTypeFilesystem,
                Description: fmt.Sprintf("Host bind mount %s needs conversion", mount.Source),
                Required:    true,
                AutoFix:     false,
            })
            plan.RiskLevel = RiskLevelMedium
        }
    }
    
    // Check for host networking
    if info.HostConfig.NetworkMode == "host" {
        plan.RequiredChanges = append(plan.RequiredChanges, MigrationChange{
            Type:        ChangeTypeNetwork,
            Description: "Host network mode not supported in microVM",
            Required:    true,
            AutoFix:     false,
        })
        plan.RiskLevel = RiskLevelHigh
    }
    
    // 3. Determine target template
    plan.TargetTemplate = d.selectTemplate(info.Config.Image)
    
    return plan, nil
}

// ContainerdAdapter wraps containerd
type ContainerdAdapter struct {
    client *containerd.Client
}

// GVisorAdapter wraps gVisor runtime
type GVisorAdapter struct {
    runtime *gvisor.Runtime
}
```

---

## Implementation Priority

### High Priority (Phase 4)
1. **Phlegethon** - Critical for workload classification
2. **Typhon** - Essential for security
3. **Hypnos** - Enables cost optimization

### Medium Priority (Phase 5)
4. **Cerberus** - Required for production auth
5. **Charon** - Needed for HA deployments
6. **Thanatos** - Important for graceful operations

### Lower Priority (Phase 6)
7. **Persephone** - Advanced scaling optimization
8. **Kampe** - Migration tooling for adoption

---

## Conclusion

These new Chthonic powers extend Tartarus's capabilities from basic sandbox execution to a complete, production-grade platform. Each component draws from Greek mythology not just for naming, but for conceptual clarity—the metaphors help developers understand the system's purpose and behavior.

The Underworld grows more complete. The dead shall be judged, the wicked punished, and the worthy granted passage.

*"Even the gods respect the laws of the Underworld."*
