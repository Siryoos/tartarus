# Tartarus: The Underworld Operating System

## A Technical Roadmap for Hyperscale MicroVM Orchestration

> *"Below even Hades lies Tartarus—as far beneath the earth as the sky is above it. Here the Titans are bound in darkness, guarded by the Hundred-Handed Ones behind gates of bronze."*
> — Hesiod, Theogony

---

## Table of Contents

1. [Cosmology: System Architecture Overview](#cosmology-system-architecture-overview)
2. [The Pantheon: Component Reference](#the-pantheon-component-reference)
3. [Phase 0: The Primordial Chaos](#phase-0-the-primordial-chaos)
4. [Phase 1: Forging the Bronze Gates](#phase-1-forging-the-bronze-gates)
5. [Phase 2: The Rivers Flow](#phase-2-the-rivers-flow)
6. [Phase 3: The Judges Take Their Thrones](#phase-3-the-judges-take-their-thrones)
7. [Phase 4: The Titans Awaken](#phase-4-the-titans-awaken)
8. [Phase 5: Ascension to Olympus](#phase-5-ascension-to-olympus)
9. [Phase 6: The Golden Age](#phase-6-the-golden-age)
10. [Technical Specifications](#technical-specifications)
11. [Appendix: Extended Mythology Mapping](#appendix-extended-mythology-mapping)

---

## Cosmology: System Architecture Overview

Tartarus implements a three-realm architecture reflecting the Greek cosmological hierarchy:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              OLYMPUS                                        │
│                         (Control Plane)                                     │
│    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│    │   Themis    │  │   Moirai    │  │   Hermes    │  │   Judges    │      │
│    │  (Policy)   │  │ (Scheduler) │  │ (Telemetry) │  │ (Admission) │      │
│    └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘      │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                               HADES                                         │
│                      (Cluster Resource Plane)                               │
│    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│    │   Acheron   │  │   Cocytus   │  │    Styx     │  │ Phlegethon  │      │
│    │   (Queue)   │  │  (Errors)   │  │  (Network)  │  │  (Hot Path) │      │
│    └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘      │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TARTARUS                                       │
│                         (Sandbox Realm)                                     │
│    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│    │Hecatoncheir │  │     Nyx     │  │   Erebus    │  │    Lethe    │      │
│    │  (Agent)    │  │ (Snapshots) │  │  (Storage)  │  │ (Ephemeral) │      │
│    └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘      │
│                                                                             │
│    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                       │
│    │   Erinyes   │  │   Typhon    │  │   Kampe     │                       │
│    │  (Enforce)  │  │(Quarantine) │  │  (Legacy)   │                       │
│    └─────────────┘  └─────────────┘  └─────────────┘                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### The Great Chasm: Data Flow

```
                    ┌──────────────────┐
                    │  Mortal Request  │
                    │   (API Call)     │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │     OLYMPUS      │
                    │   (Validation)   │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │  Minos   │  │Rhadaman- │  │  Aeacus  │
        │ (Quota)  │  │  thus    │  │ (Audit)  │
        └────┬─────┘  │(Security)│  └────┬─────┘
             │        └────┬─────┘       │
             └──────────────┼────────────┘
                           │
                    ┌──────┴──────┐
                    │   Verdict   │
                    │Accept/Reject│
                    └──────┬──────┘
                           │
                           ▼
                    ┌──────────────────┐
                    │     ACHERON      │
                    │  (Job Queue)     │
                    │ "River of Pain"  │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │     MOIRAI       │
                    │   (Scheduler)    │
                    │ "Weavers of Fate"│
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │   HECATONCHEIR   │
                    │  (Node Agent)    │
                    │ "Hundred-Handed" │
                    └────────┬─────────┘
                             │
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                  ▼
    ┌──────────┐      ┌──────────┐      ┌──────────┐
    │   NYX    │      │  LETHE   │      │   STYX   │
    │(Snapshot)│      │(Overlay) │      │(Network) │
    └────┬─────┘      └────┬─────┘      └────┬─────┘
         └──────────────────┼──────────────────┘
                           │
                           ▼
                    ┌──────────────────┐
                    │    MICROVM       │
                    │   (Firecracker)  │
                    │ "The Prisoner"   │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ ERINYES  │  │  HERMES  │  │  Normal  │
        │(Enforce) │  │ (Logs)   │  │  Exit    │
        │ Timeout  │  │ Stream   │  │          │
        └────┬─────┘  └────┬─────┘  └────┬─────┘
             │              │              │
             └──────────────┼──────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
        ┌──────────┐            ┌──────────┐
        │ COCYTUS  │            │  LETHE   │
        │(Failures)│            │ (Forget) │
        │"Wailing" │            │ (Clean)  │
        └──────────┘            └──────────┘
```

---

## The Pantheon: Component Reference

### Olympian Tier (Control Plane)

| Deity | Package | Purpose | Status |
|-------|---------|---------|--------|
| **Olympus** | `pkg/olympus` | API Gateway & Central Manager | ✅ Implemented |
| **Themis** | `pkg/themis` | Policy Definition & Validation | ✅ Implemented |
| **Moirai** | `pkg/moirai` | Fate/Scheduler (node selection) | ✅ Implemented |
| **Hermes** | `pkg/hermes` | Telemetry & Log Shipping | ✅ Implemented |
| **Judges** | `pkg/judges` | Admission Control (Minos, Rhadamanthus, Aeacus) | ✅ Implemented |

### Chthonic Tier (Cluster Resources)

| River/Entity | Package | Purpose | Status |
|--------------|---------|---------|--------|
| **Hades** | `pkg/hades` | Node Registry & Cluster View | ✅ Implemented |
| **Acheron** | `pkg/acheron` | Job Queue (River of Pain/Ingress) | ✅ Implemented |
| **Styx** | `pkg/styx` | Network Gateway (River of Oaths) | ✅ Implemented |
| **Cocytus** | `pkg/cocytus` | Error Stream (River of Wailing) | ✅ Implemented |
| **Phlegethon** | `pkg/phlegethon` | Hot Path Routing (River of Fire) | ✅ Implemented |

### Tartarean Tier (Sandbox Execution)

| Entity | Package | Purpose | Status |
|--------|---------|---------|--------|
| **Hecatoncheir** | `pkg/hecatoncheir` | Node Agent (Hundred-Handed Guardian) | ✅ Implemented |
| **Nyx** | `pkg/nyx` | Snapshot Manager (Primordial Night) | ✅ Implemented |
| **Erebus** | `pkg/erebus` | Deep Storage (Primordial Darkness) | ✅ Implemented |
| **Lethe** | `pkg/lethe` | Ephemeral FS (River of Forgetting) | ✅ Implemented |
| **Erinyes** | `pkg/erinyes` | Enforcement/Punishment (The Furies) | ✅ Implemented |
| **Tartarus** | `pkg/tartarus` | MicroVM Runtime Interface | ✅ Implemented |
| **Typhon** | `pkg/typhon` | Quarantine Pool (Monster of Chaos) | 🔲 Planned |
| **Kampe** | `pkg/kampe` | Legacy Runtime Shim (Old Jailor) | 🔲 Planned |

### New Entities (Phase 4+)

| Entity | Package | Purpose | Status |
|--------|---------|---------|--------|
| **Hypnos** | `pkg/hypnos` | Sleep/Hibernation Manager | 🔲 Planned |
| **Thanatos** | `pkg/thanatos` | Graceful Termination Handler | 🔲 Planned |
| **Charon** | `pkg/charon` | Request Ferry/Load Balancer | 🔲 Planned |
| **Cerberus** | `pkg/cerberus` | Authentication Gateway (Three-Headed Guard) | 🔲 Planned |
| **Persephone** | `pkg/persephone` | Seasonal Scaling (Queen of Cycles) | 🔲 Planned |

---

## Phase 0: The Primordial Chaos

**Theme:** *"In the beginning, there was Chaos—the yawning void from which all things emerged."*

**Status:** ✅ COMPLETE

**Objective:** Establish the foundational domain model, naming conventions, and project structure that will guide all subsequent development.

### Deliverables

#### 0.1 Architecture RFC
- [x] Define the three-realm architecture (Olympus, Hades, Tartarus)
- [x] Document the mythological mapping for all components
- [x] Create sequence diagrams for core flows
- [x] Establish Go module structure and package layout

#### 0.2 Go Module Skeleton
```
tartarus/
├── cmd/
│   ├── olympus-api/          # Control plane API server
│   ├── hecatoncheir-agent/   # Node agent
│   └── tartarus/             # CLI tool
├── pkg/
│   ├── olympus/              # API, manager, middleware
│   ├── hades/                # Cluster registry
│   ├── tartarus/             # Runtime interface
│   ├── hecatoncheir/         # Agent implementation
│   ├── nyx/                  # Snapshot management
│   ├── erebus/               # Blob storage
│   ├── styx/                 # Network gateway
│   ├── lethe/                # Ephemeral filesystem
│   ├── judges/               # Admission control
│   ├── erinyes/              # Enforcement
│   ├── acheron/              # Job queue
│   ├── cocytus/              # Error handling
│   ├── hermes/               # Observability
│   ├── moirai/               # Scheduling
│   ├── themis/               # Policy engine
│   └── domain/               # Core types
└── build/
    ├── Dockerfile.agent
    └── Dockerfile.olympus
```

#### 0.3 Core Interfaces

All interfaces have been defined in their respective packages:

```go
// Runtime abstraction (pkg/tartarus/runtime.go)
type Runtime interface {
    Create(ctx context.Context, cfg *VMConfig) (string, error)
    Start(ctx context.Context, vmID string) error
    Stop(ctx context.Context, vmID string) error
    Status(ctx context.Context, vmID string) (*VMStatus, error)
    Logs(ctx context.Context, vmID string) (io.ReadCloser, error)
}

// Snapshot management (pkg/nyx/manager.go)
type Manager interface {
    Create(ctx context.Context, template TemplateID) (*Snapshot, error)
    Restore(ctx context.Context, snapshotID string) (*RestoredVM, error)
    List(ctx context.Context) ([]*Snapshot, error)
    Delete(ctx context.Context, snapshotID string) error
}

// Network gateway (pkg/styx/gateway.go)
type Gateway interface {
    CreateContract(vmID string, policy NetworkPolicy) error
    EnforceContract(vmID string) error
    RevokeContract(vmID string) error
}

// Scheduler (pkg/moirai/scheduler.go)
type Scheduler interface {
    Schedule(ctx context.Context, req *SandboxRequest) (*NodeAssignment, error)
    Rebalance(ctx context.Context) error
}

// Judge (pkg/judges/judge.go)
type Judge interface {
    PreAdmit(ctx context.Context, req *SandboxRequest) (Verdict, error)
    PostHoc(ctx context.Context, run *SandboxRun) (*Classification, error)
}
```

---

## Phase 1: Forging the Bronze Gates

**Theme:** *"The Hecatoncheires—Briareus, Cottus, and Gyes—stand eternal watch at the bronze gates of Tartarus, their hundred hands ready to restrain any who would escape."*

**Status:** ✅ COMPLETE

**Objective:** Deliver a minimal single-node prototype capable of launching MicroVMs from snapshots with sub-second startup times.

### Deliverables

#### 1.1 Hecatoncheir Agent v1.0
- [x] gRPC/HTTP API for sandbox lifecycle
- [x] Firecracker integration via API socket
- [x] Process supervision and cleanup
- [x] Basic resource limits via cgroups

```go
// pkg/hecatoncheir/agent.go
type Agent struct {
    ID           string
    NodeInfo     NodeInfo
    Runtime      tartarus.Runtime
    SnapshotMgr  nyx.Manager
    EphemeralFS  lethe.Pool
    Gateway      styx.Gateway
    Metrics      hermes.Emitter
}

func (a *Agent) LaunchSandbox(ctx context.Context, req *LaunchRequest) (*Sandbox, error)
func (a *Agent) GetStatus(ctx context.Context, sandboxID string) (*SandboxStatus, error)
func (a *Agent) Kill(ctx context.Context, sandboxID string) error
func (a *Agent) StreamLogs(ctx context.Context, sandboxID string) (<-chan []byte, error)
```

#### 1.2 Nyx Snapshot Manager v1.0
- [x] Build minimal rootfs images (Alpine + Python/Node)
- [x] Snapshot creation from running VM
- [x] Snapshot restoration with COW memory mapping
- [x] Local filesystem storage backend

```go
// pkg/nyx/local_manager.go
type LocalManager struct {
    store      erebus.Store
    kernelPath string
    cache      map[TemplateID]*Snapshot
}

func (m *LocalManager) WarmUp(ctx context.Context, template TemplateID) error {
    // 1. Boot VM from base image
    // 2. Run initialization script (import libraries, etc.)
    // 3. Pause VM and capture snapshot
    // 4. Store snapshot in Erebus
}
```

#### 1.3 Lethe Ephemeral Filesystem v1.0
- [x] Overlay filesystem creation per sandbox
- [x] Copy-on-write from base snapshot
- [x] Automatic cleanup on sandbox termination
- [x] Secure shredding option for sensitive workloads

#### 1.4 Tartarus CLI v1.0
- [x] `tartarus run --template <name> -- <command>`
- [x] `tartarus ps` - List running sandboxes
- [x] `tartarus logs <sandbox-id>` - Stream logs
- [x] `tartarus kill <sandbox-id>` - Terminate sandbox

### Milestone Metrics
| Metric | Target | Achieved |
|--------|--------|----------|
| Cold start (from snapshot) | < 100ms | 🔄 Testing |
| Memory overhead per VM | < 10MB | 🔄 Testing |
| Concurrent VMs per host | > 100 | 🔄 Testing |

---

## Phase 2: The Rivers Flow

**Theme:** *"Five rivers wind through the Underworld: Acheron, the river of pain; Styx, the river of oaths; Lethe, the river of forgetting; Phlegethon, the river of fire; and Cocytus, the river of wailing."*

**Status:** ✅ COMPLETE

**Objective:** Implement the data flow infrastructure—queuing, networking, error handling—that connects all system components.

### Deliverables

#### 2.1 Acheron Queue v1.0
- [x] In-memory queue for development
- [x] Redis-backed queue for production
- [x] Priority lanes (standard, expedited, bulk)
- [x] Backpressure handling

```go
// pkg/acheron/queue.go
type Queue interface {
    Enqueue(ctx context.Context, req *SandboxRequest) error
    Dequeue(ctx context.Context) (*SandboxRequest, error)
    Acknowledge(ctx context.Context, reqID string) error
    Reject(ctx context.Context, reqID string, reason string) error
    Len() int
}

// Priority lanes based on mythological river branches
const (
    LaneStandard  Lane = "main"      // Normal requests
    LaneExpedited Lane = "swift"     // High-priority
    LaneBulk      Lane = "deep"      // Batch processing
)
```

#### 2.2 Styx Network Gateway v1.0
- [x] TAP device creation per MicroVM
- [x] Bridge/NAT configuration
- [x] iptables/nftables rule generation
- [x] Contract-based policy enforcement

```go
// pkg/styx/gateway.go
type Contract struct {
    VMID         string
    AllowedCIDRs []net.IPNet
    DeniedCIDRs  []net.IPNet
    DenyPrivate  bool              // Block RFC1918
    DenyMetadata bool              // Block 169.254.169.254
    DenyDNS      bool              // Block DNS (for strict isolation)
    AllowedPorts []int             // Egress port whitelist
    RateLimit    *RateLimitConfig  // Bandwidth/packet limits
}

// The Oath: Once a contract is sworn on the Styx, it cannot be broken
func (g *HostGateway) SwearOath(contract *Contract) error {
    // 1. Create TAP device
    // 2. Configure bridge attachment
    // 3. Apply iptables rules
    // 4. Record oath in registry
}
```

#### 2.3 Cocytus Error Stream v1.0
- [x] Structured error records
- [x] Dead-letter queue for failed sandboxes
- [x] Error classification and tagging
- [x] Retention policy enforcement

```go
// pkg/cocytus/sink.go
type Lamentation struct {
    SandboxID   string
    Timestamp   time.Time
    Category    ErrorCategory
    Message     string
    StackTrace  string
    Metadata    map[string]string
}

type Sink interface {
    Wail(ctx context.Context, lament *Lamentation) error
    Query(ctx context.Context, filter *LamentFilter) ([]*Lamentation, error)
    Purge(ctx context.Context, olderThan time.Duration) error
}
```

#### 2.4 Hermes Observability v1.0
- [x] Prometheus metrics exporter
- [x] Structured logging with context propagation
- [x] Log aggregation per sandbox
- [x] Trace span integration (OpenTelemetry)

```go
// pkg/hermes/observability.go
type Emitter interface {
    // Metrics
    RecordLaunchLatency(template string, duration time.Duration)
    RecordSnapshotRestoreTime(snapshotID string, duration time.Duration)
    IncrementSandboxCount(template string)
    DecrementSandboxCount(template string)
    RecordResourceUsage(sandboxID string, cpu, memory float64)
    
    // Logging
    LogEvent(ctx context.Context, level Level, msg string, fields ...Field)
    
    // Tracing
    StartSpan(ctx context.Context, name string) (context.Context, Span)
}
```

### Key Metrics Exposed
```
# Sandbox lifecycle
tartarus_sandbox_launch_duration_seconds{template, node}
tartarus_sandbox_active_count{template, node}
tartarus_sandbox_total{template, status}

# Snapshot operations
tartarus_snapshot_restore_duration_seconds{snapshot_id}
tartarus_snapshot_cache_hits_total{template}
tartarus_snapshot_cache_misses_total{template}

# Network (Styx)
tartarus_styx_contracts_active{node}
tartarus_styx_packets_blocked_total{reason}
tartarus_styx_bytes_transferred_total{direction}

# Queue (Acheron)
tartarus_acheron_queue_depth{lane}
tartarus_acheron_wait_duration_seconds{lane}

# Errors (Cocytus)
tartarus_cocytus_lamentations_total{category}
```

---

## Phase 3: The Judges Take Their Thrones

**Theme:** *"Three judges preside over the dead: Minos, who holds the casting vote; Rhadamanthus, who judges those from Asia; and Aeacus, who judges those from Europe. Together they determine each soul's fate."*

**Status:** ✅ COMPLETE

**Objective:** Implement policy-driven admission control, multi-node orchestration, and active enforcement mechanisms.

### Deliverables

#### 3.1 Hades Cluster Registry v1.0
- [x] Node registration and heartbeat
- [x] Capacity tracking (CPU, memory, GPU)
- [x] Label/taint support for node selection
- [x] Health monitoring and automatic deregistration

```go
// pkg/hades/registry.go
type Registry interface {
    Register(ctx context.Context, node *NodeInfo) error
    Deregister(ctx context.Context, nodeID string) error
    Heartbeat(ctx context.Context, nodeID string, status *NodeStatus) error
    List(ctx context.Context, filter *NodeFilter) ([]*NodeInfo, error)
    Get(ctx context.Context, nodeID string) (*NodeInfo, error)
}

type NodeInfo struct {
    ID          string
    Address     string
    Capacity    Resources
    Available   Resources
    Labels      map[string]string
    Taints      []Taint
    LastSeen    time.Time
    Status      NodeStatus
}

// Special labels for mythological routing
const (
    LabelTyphon     = "tartarus.io/typhon"      // Quarantine-capable
    LabelPhlegethon = "tartarus.io/phlegethon"  // High-compute
    LabelElysium    = "tartarus.io/elysium"     // Trusted workloads
)
```

#### 3.2 Moirai Scheduler v1.0
- [x] Least-loaded node selection
- [x] Label/taint filtering
- [x] Resource reservation
- [x] Affinity/anti-affinity rules
- [x] Bin-packing optimization

```go
// pkg/moirai/scheduler.go
type Scheduler interface {
    Schedule(ctx context.Context, req *SandboxRequest) (*NodeAssignment, error)
    Reserve(ctx context.Context, nodeID string, resources Resources) error
    Release(ctx context.Context, nodeID string, resources Resources) error
}

// The three Fates: Clotho (spinner), Lachesis (allotter), Atropos (inevitable)
type LeastLoadedScheduler struct {
    registry hades.Registry
    
    // Clotho: Spins the thread of fate (creates assignments)
    // Lachesis: Measures the thread (evaluates node fitness)
    // Atropos: Cuts the thread (finalizes decision)
}
```

#### 3.3 Judges v1.0
- [x] MinosJudge: Quota and resource validation
- [x] RhadamanthusJudge: Security policy enforcement
- [x] AeacusJudge: Compliance tagging and audit
- [x] Custom judge plugin interface

```go
// pkg/judges/basic_judges.go

// Minos: The quota keeper - validates resource requests against limits
type MinosJudge struct {
    quotaStore QuotaStore
}

func (m *MinosJudge) PreAdmit(ctx context.Context, req *SandboxRequest) (Verdict, error) {
    // Check tenant quotas
    // Validate resource request bounds
    // Verify template exists and is allowed
}

// Rhadamanthus: The security arbiter - enforces security policies
type RhadamanthusJudge struct {
    policyStore themis.Repository
}

func (r *RhadamanthusJudge) PreAdmit(ctx context.Context, req *SandboxRequest) (Verdict, error) {
    // Check network policy compatibility
    // Validate no dangerous flag combinations
    // Verify caller identity and permissions
}

// Aeacus: The record keeper - tags for compliance and audit
type AeacusJudge struct {
    auditLog AuditLogger
}

func (a *AeacusJudge) PreAdmit(ctx context.Context, req *SandboxRequest) (Verdict, error) {
    // Record request details
    // Apply compliance tags
    // Determine retention requirements
}
```

#### 3.4 Erinyes Enforcement v1.0
- [x] Timeout enforcement (Alecto - "unceasing")
- [x] Resource limit enforcement (Tisiphone - "avenger")
- [x] Policy violation detection (Megaera - "grudging")
- [x] Automated quarantine triggers

```go
// pkg/erinyes/fury.go
type Fury interface {
    Watch(ctx context.Context, sandboxID string, policy EnforcementPolicy) error
    Unwatch(sandboxID string) error
}

type EnforcementPolicy struct {
    MaxDuration     time.Duration
    MaxCPUPercent   float64
    MaxMemoryBytes  int64
    NetworkPolicy   *styx.Contract
    OnViolation     ViolationAction  // Warn, Throttle, Kill, Quarantine
}

// The three Furies
// Alecto: "Unceasing" - timeout and duration enforcement
// Tisiphone: "Avenger of murder" - resource limit enforcement  
// Megaera: "Grudging" - policy violation detection

type PollFury struct {
    runtime     tartarus.Runtime
    gateway     styx.Gateway
    errorSink   cocytus.Sink
    watchList   map[string]*WatchEntry
}

func (f *PollFury) unleashAlecto(sandboxID string, maxDuration time.Duration) {
    // Start timeout timer
    // On expiry, terminate sandbox
    // Record in Cocytus
}

func (f *PollFury) unleashTisiphone(sandboxID string, limits ResourceLimits) {
    // Poll resource usage
    // On violation, apply action (throttle/kill)
    // Record in Cocytus
}

func (f *PollFury) unleashMegaera(sandboxID string, policy *styx.Contract) {
    // Monitor network violations
    // Detect policy breaches
    // Record in Cocytus
}
```

#### 3.5 Themis Policy Engine v1.0
- [x] YAML/JSON policy definitions
- [x] Template-level defaults
- [x] Tenant-level overrides
- [x] Policy versioning and rollback

```go
// pkg/themis/policy.go
type Policy struct {
    ID          string            `yaml:"id"`
    Name        string            `yaml:"name"`
    Version     int               `yaml:"version"`
    Templates   []TemplatePolicy  `yaml:"templates"`
    Network     NetworkPolicy     `yaml:"network"`
    Resources   ResourcePolicy    `yaml:"resources"`
    Enforcement EnforcementPolicy `yaml:"enforcement"`
}

type TemplatePolicy struct {
    ID              string   `yaml:"id"`
    AllowedCallers  []string `yaml:"allowed_callers"`
    MaxInstances    int      `yaml:"max_instances"`
    DefaultTimeout  string   `yaml:"default_timeout"`
}

// Example policy YAML:
/*
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
*/
```

### Milestone Metrics
| Metric | Target | Status |
|--------|--------|--------|
| Multi-node orchestration | 10+ nodes | ✅ Verified |
| Admission latency | < 10ms | ✅ Verified |
| Policy evaluation time | < 5ms | ✅ Verified |
| Enforcement accuracy | 99.9% | ✅ Verified |

---

## Phase 4: The Titans Awaken

**Theme:** *"Deep within Tartarus, the Titans stir—ancient powers of chaos and creation, bound yet never truly defeated. Their fire still burns beneath the earth."*

**Status:** 🔲 PLANNED

**Objective:** Enable data science and AI workloads with specialized templates, GPU support preparation, and advanced resource management.

### Deliverables

#### 4.1 OCI Image Pipeline (Erebus v2.0)
- [ ] OCI image pulling and layer extraction
- [ ] Rootfs construction from container images
- [ ] Layer caching and deduplication
- [ ] Integration with container registries (Docker Hub, GCR, ECR)

```go
// pkg/erebus/oci_builder.go
type OCIBuilder interface {
    Pull(ctx context.Context, ref string, auth *RegistryAuth) error
    BuildRootFS(ctx context.Context, ref string, outputPath string) error
    CacheStatus(ref string) (*CacheEntry, error)
}

// Convert any Docker image to a Tartarus-bootable rootfs
func (b *LocalOCIBuilder) BuildRootFS(ctx context.Context, ref string, output string) error {
    // 1. Pull image manifest
    // 2. Extract layers in order
    // 3. Apply tar overlays
    // 4. Inject Tartarus init system
    // 5. Output bootable disk image
}
```

#### 4.2 Nyx Data Science Snapshots
- [ ] Pre-built Python DS template (NumPy, Pandas, SciPy, Matplotlib)
- [ ] Pre-built ML template (PyTorch, TensorFlow, scikit-learn)
- [ ] Pre-built R Analytics template
- [ ] Julia Scientific Computing template
- [ ] Snapshot warming automation

```yaml
# templates/python-ds.yaml
id: python-ds
name: Python Data Science
base_image: python:3.11-slim
warmup_script: |
  import numpy
  import pandas
  import scipy
  import matplotlib
  print("Libraries loaded")
resources:
  default_cpu: 2
  default_memory: 4096
  max_cpu: 8
  max_memory: 32768
```

#### 4.3 Phlegethon Hot Path Router
- [ ] Workload classification (light/medium/heavy)
- [ ] Dedicated node pools for heavy workloads
- [ ] Resource class definitions
- [ ] Automatic routing based on request characteristics

```go
// pkg/phlegethon/router.go
type HeatLevel string

const (
    HeatCold   HeatLevel = "cold"    // Quick, light tasks
    HeatWarm   HeatLevel = "warm"    // Standard workloads
    HeatHot    HeatLevel = "hot"     // CPU-intensive
    HeatInferno HeatLevel = "inferno" // GPU/long-running
)

type Router interface {
    ClassifyHeat(req *SandboxRequest) HeatLevel
    RouteToPool(heat HeatLevel) (string, error)
}

// Resource classes (the flames of Phlegethon)
type ResourceClass struct {
    Name        string
    CPUCores    int
    MemoryMB    int
    GPUCount    int
    MaxDuration time.Duration
    NodeLabels  map[string]string
}

var (
    ClassEmber  = ResourceClass{Name: "ember", CPUCores: 1, MemoryMB: 512}
    ClassFlame  = ResourceClass{Name: "flame", CPUCores: 2, MemoryMB: 2048}
    ClassBlaze  = ResourceClass{Name: "blaze", CPUCores: 4, MemoryMB: 8192}
    ClassInferno = ResourceClass{Name: "inferno", CPUCores: 8, MemoryMB: 32768}
)
```

#### 4.4 Typhon Quarantine Pool
- [ ] Dedicated high-isolation node pool
- [ ] Enhanced seccomp profiles
- [ ] No network access by default
- [ ] Separate storage backend
- [ ] Automatic classification of suspicious workloads

```go
// pkg/typhon/quarantine.go

// Typhon: The monster bound in Tartarus, source of volcanic fire
// In our system: the quarantine zone for dangerous workloads

type QuarantinePool struct {
    nodes      []string              // Dedicated quarantine nodes
    seccomp    *SeccompProfile       // Hardened syscall filter
    networking NetworkMode           // None, or very restricted
    storage    erebus.Store          // Isolated storage backend
}

type QuarantineRequest struct {
    SandboxID   string
    Reason      QuarantineReason
    OriginalReq *SandboxRequest
    Evidence    []byte
}

type QuarantineReason string

const (
    ReasonSuspiciousBehavior QuarantineReason = "suspicious_behavior"
    ReasonPolicyViolation    QuarantineReason = "policy_violation"
    ReasonUntrustedSource    QuarantineReason = "untrusted_source"
    ReasonManualFlag         QuarantineReason = "manual_flag"
)
```

#### 4.5 Hypnos Sleep Manager (NEW)
- [ ] VM hibernation to disk
- [ ] Memory compression before sleep
- [ ] Quick wake-up from hibernation
- [ ] Cost optimization through sleep cycles

```go
// pkg/hypnos/sleep.go

// Hypnos: God of Sleep, twin brother of Thanatos
// Manages VM hibernation for cost optimization

type SleepManager interface {
    Sleep(ctx context.Context, sandboxID string) error
    Wake(ctx context.Context, sandboxID string) error
    IsSleeping(sandboxID string) bool
    ScheduleSleep(sandboxID string, after time.Duration) error
}

type HypnosManager struct {
    runtime      tartarus.Runtime
    snapshotMgr  nyx.Manager
    sleepStore   erebus.Store
    wakeTriggers map[string]chan struct{}
}

// Hibernation flow:
// 1. Pause VM
// 2. Compress memory pages
// 3. Write to sleep store
// 4. Release resources
// 5. On wake: restore from sleep snapshot
```

### Milestone Metrics
| Metric | Target | Status |
|--------|--------|--------|
| DS template cold start | < 200ms | 🔲 Not started |
| OCI image conversion | < 30s | 🔲 Not started |
| Quarantine latency | < 50ms | 🔲 Not started |
| Hibernation restore | < 100ms | 🔲 Not started |

---

## Phase 5: Ascension to Olympus

**Theme:** *"From the depths of Tartarus, one may ascend through the realms of Hades, past the judgment seat, across the rivers, and finally emerge into the light of Olympus—if deemed worthy."*

**Status:** 🔲 PLANNED

**Objective:** Production hardening, developer experience improvements, and ecosystem integration.

### Deliverables

#### 5.1 Cerberus Authentication Gateway (NEW)
- [ ] API key authentication
- [ ] OAuth2/OIDC integration
- [ ] mTLS for agent communication
- [ ] Role-based access control (RBAC)

```go
// pkg/cerberus/auth.go

// Cerberus: Three-headed hound guarding the gates of Hades
// Our authentication and authorization gateway

type AuthGateway interface {
    // The three heads of Cerberus:
    // 1. Authentication (identity verification)
    Authenticate(ctx context.Context, creds Credentials) (*Identity, error)
    
    // 2. Authorization (permission checking)
    Authorize(ctx context.Context, identity *Identity, action Action) error
    
    // 3. Audit (access logging)
    RecordAccess(ctx context.Context, identity *Identity, action Action, result Result) error
}

type Identity struct {
    ID          string
    Type        IdentityType  // User, Service, Agent
    Tenant      string
    Roles       []string
    Permissions []Permission
}

type Permission string

const (
    PermissionSandboxCreate  Permission = "sandbox:create"
    PermissionSandboxRead    Permission = "sandbox:read"
    PermissionSandboxDelete  Permission = "sandbox:delete"
    PermissionTemplateManage Permission = "template:manage"
    PermissionPolicyManage   Permission = "policy:manage"
    PermissionAdminAll       Permission = "admin:*"
)
```

#### 5.2 Charon Request Ferry (NEW)
- [ ] Load balancing across Olympus instances
- [ ] Request routing and failover
- [ ] Rate limiting per tenant
- [ ] Circuit breaker patterns

```go
// pkg/charon/ferry.go

// Charon: Ferryman of Hades who carries souls across the Styx
// Our load balancer and request router

type Ferry interface {
    // Ferry a request to an appropriate handler
    Cross(ctx context.Context, req *Request) (*Response, error)
    
    // Register a destination (Olympus instance)
    RegisterShore(addr string, weight int) error
    
    // Health checking
    CheckPassage(addr string) error
}

type FerryConfig struct {
    Shores          []ShoreConfig       // Backend instances
    LoadBalancing   LoadBalanceStrategy // RoundRobin, LeastConn, Weighted
    CircuitBreaker  CircuitBreakerConfig
    RateLimiting    RateLimitConfig
    RetryPolicy     RetryConfig
}
```

#### 5.3 Tartarus CLI v2.0
- [ ] `tartarus init template` - Create new template from Dockerfile
- [ ] `tartarus snapshot create/list/delete` - Snapshot management
- [ ] `tartarus logs --follow` - Live log streaming
- [ ] `tartarus exec <sandbox-id> -- <command>` - Execute in running sandbox
- [ ] `tartarus inspect <sandbox-id>` - Detailed sandbox info
- [ ] `tartarus config` - Configuration management
- [ ] Tab completion for bash/zsh

```bash
# Example CLI session
$ tartarus init template --name my-python --from python:3.11
Creating template 'my-python' from python:3.11...
✓ Pulled image
✓ Built rootfs
✓ Created snapshot
Template 'my-python' ready (ID: tpl-abc123)

$ tartarus run --template my-python -- python -c "print('Hello from Tartarus')"
Sandbox launched (ID: sbx-xyz789)
Hello from Tartarus
Sandbox completed (exit code: 0, duration: 127ms)

$ tartarus ps
ID           TEMPLATE     STATUS    STARTED          DURATION
sbx-abc123   python-ds    running   2 minutes ago    2m13s
sbx-def456   node-18      running   5 minutes ago    5m02s

$ tartarus logs sbx-abc123 --follow
[2025-01-15T10:30:00Z] Processing data...
[2025-01-15T10:30:01Z] Analysis complete.
^C

$ tartarus kill sbx-abc123
Sandbox sbx-abc123 terminated
```

#### 5.4 Observability Dashboard
- [ ] Grafana dashboard templates
- [ ] Real-time sandbox visualization
- [ ] Resource usage heatmaps
- [ ] Error rate and latency graphs
- [ ] Capacity planning views

#### 5.5 Security Hardening
- [ ] Guest kernel hardening (grsecurity-inspired)
- [ ] Seccomp profile generator
- [ ] Automatic vulnerability scanning of templates
- [ ] Secrets injection via Vault/KMS integration

```go
// pkg/olympus/security.go

type SecurityConfig struct {
    // Kernel hardening
    HardenedKernel    bool   // Use hardened guest kernel
    KernelLockdown    string // none, integrity, confidentiality
    
    // Seccomp
    SeccompProfile    string // Path to custom profile
    SeccompDefault    string // kill, trap, errno
    
    // Capabilities
    DropCapabilities  []string
    AddCapabilities   []string
    
    // User namespace
    UserNamespace     bool
    UIDMapping        []IDMapping
    GIDMapping        []IDMapping
}
```

#### 5.6 Kubernetes Integration
- [ ] CRI (Container Runtime Interface) shim
- [ ] Custom Resource Definitions (SandboxJob, SandboxTemplate)
- [ ] Operator for lifecycle management
- [ ] Pod Security Policy integration

```yaml
# Example Kubernetes CRD
apiVersion: tartarus.io/v1alpha1
kind: SandboxJob
metadata:
  name: data-analysis-job
  namespace: ml-workloads
spec:
  template: python-ds
  command: ["python", "/app/analyze.py"]
  timeout: 10m
  resources:
    cpu: 4
    memory: 8Gi
  network:
    policy: restricted
  storage:
    inputs:
      - name: dataset
        source: s3://bucket/data.csv
    outputs:
      - name: results
        destination: s3://bucket/results/
```

### Milestone Metrics
| Metric | Target | Status |
|--------|--------|--------|
| API availability | 99.9% | 🔲 Not started |
| Authentication latency | < 5ms | 🔲 Not started |
| Dashboard load time | < 2s | 🔲 Not started |
| K8s integration e2e | < 1s overhead | 🔲 Not started |

---

## Phase 6: The Golden Age

**Theme:** *"Under the reign of Kronos, before Zeus and the Olympians, there was a Golden Age—a time of peace and plenty. We seek to bring that age to infrastructure."*

**Status:** 🔲 FUTURE

**Objective:** Advanced features, ecosystem expansion, and community building for widespread adoption.

### Deliverables

#### 6.1 Persephone Seasonal Scaling (NEW)
- [ ] Predictive autoscaling based on historical patterns
- [ ] Scheduled scale-up/down (like Persephone's seasons)
- [ ] Cost optimization through intelligent hibernation
- [ ] Integration with cloud provider scaling

```go
// pkg/persephone/seasons.go

// Persephone: Queen of the Underworld, goddess of seasonal change
// Manages predictive scaling and seasonal patterns

type SeasonalScaler interface {
    // Predict demand for the coming period
    Forecast(ctx context.Context, window time.Duration) (*Forecast, error)
    
    // Apply seasonal scaling rules
    ApplySeason(ctx context.Context, season *Season) error
    
    // Learn from historical patterns
    Learn(ctx context.Context, history []*UsageRecord) error
}

type Season struct {
    Name        string        // e.g., "peak-hours", "overnight"
    Start       time.Time
    End         time.Time
    MinNodes    int
    MaxNodes    int
    TargetUtil  float64
    Prewarming  PrewarmConfig
}
```

#### 6.2 Thanatos Graceful Termination (NEW)
- [ ] Graceful shutdown signal handling
- [ ] State preservation on termination
- [ ] Checkpoint creation before death
- [ ] Clean resource release

```go
// pkg/thanatos/termination.go

// Thanatos: God of peaceful death, twin of Hypnos
// Handles graceful termination and state preservation

type TerminationHandler interface {
    // Initiate graceful shutdown
    InitiateShutdown(ctx context.Context, sandboxID string, grace time.Duration) error
    
    // Create checkpoint before termination
    Checkpoint(ctx context.Context, sandboxID string) (*Checkpoint, error)
    
    // Force termination if grace period exceeded
    ForceTerminate(ctx context.Context, sandboxID string) error
}

type TerminationReason string

const (
    ReasonCompleted     TerminationReason = "completed"
    ReasonTimeout       TerminationReason = "timeout"
    ReasonUserRequest   TerminationReason = "user_request"
    ReasonResourceLimit TerminationReason = "resource_limit"
    ReasonPolicyViolation TerminationReason = "policy_violation"
    ReasonNodeDrain     TerminationReason = "node_drain"
)
```

#### 6.3 Kampe Legacy Compatibility (NEW)
- [ ] Docker runtime adapter
- [ ] containerd shim
- [ ] gVisor integration option
- [ ] Migration tools from containers to microVMs

```go
// pkg/kampe/legacy.go

// Kampe: The ancient jailor, monster who guarded the Cyclopes
// Represents legacy runtimes we're migrating away from

type LegacyRuntime interface {
    tartarus.Runtime  // Implements same interface
    
    // Migration helpers
    MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error)
    ExportState(ctx context.Context, containerID string) (*ContainerState, error)
}

// Adapters for different legacy runtimes
type DockerAdapter struct {
    client *docker.Client
}

type ContainerdAdapter struct {
    client *containerd.Client
}

type GVisorAdapter struct {
    runtime *gvisor.Runtime
}
```

#### 6.4 Unified WASM-MicroVM Runtime
- [ ] WebAssembly runtime integration (WasmEdge/Wasmtime)
- [ ] Automatic selection based on workload
- [ ] Hybrid execution (WASM for light, MicroVM for heavy)
- [ ] Shared interface across isolation types

```go
// pkg/tartarus/unified_runtime.go

type IsolationType string

const (
    IsolationMicroVM IsolationType = "microvm"
    IsolationWASM    IsolationType = "wasm"
    IsolationGVisor  IsolationType = "gvisor"
    IsolationAuto    IsolationType = "auto"
)

type UnifiedRuntime struct {
    microvm  *FirecrackerRuntime
    wasm     *WasmRuntime
    gvisor   *GVisorRuntime
    selector IsolationSelector
}

func (r *UnifiedRuntime) Create(ctx context.Context, cfg *VMConfig) (string, error) {
    isolation := r.selector.Select(cfg)
    switch isolation {
    case IsolationMicroVM:
        return r.microvm.Create(ctx, cfg)
    case IsolationWASM:
        return r.wasm.Create(ctx, cfg)
    case IsolationGVisor:
        return r.gvisor.Create(ctx, cfg)
    default:
        return "", fmt.Errorf("unknown isolation type: %s", isolation)
    }
}
```

#### 6.5 Community & Ecosystem
- [ ] Comprehensive documentation site
- [ ] Tutorial series (blog posts, videos)
- [ ] Template marketplace
- [ ] Plugin system for custom judges/furies
- [ ] Terraform provider
- [ ] GitHub Actions integration
- [ ] VS Code extension

#### 6.6 Performance Targets (Golden Age)
| Metric | Target |
|--------|--------|
| Cold start (snapshot restore) | < 25ms |
| Warm start (from Hypnos sleep) | < 10ms |
| Concurrent VMs per host | 500+ |
| API latency (p99) | < 50ms |
| Availability | 99.99% |

---

## Technical Specifications

### System Requirements

#### Control Plane (Olympus)
| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| Memory | 2GB | 8GB |
| Storage | 10GB SSD | 50GB SSD |
| Network | 100Mbps | 1Gbps |

#### Worker Node (Hecatoncheir)
| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 4 cores | 16+ cores |
| Memory | 8GB | 64GB+ |
| Storage | 50GB SSD | 500GB NVMe |
| Network | 1Gbps | 10Gbps |
| Virtualization | KVM enabled | KVM + huge pages |

### Supported Platforms
- **Host OS:** Linux (kernel 5.10+)
- **Architectures:** x86_64, aarch64
- **Hypervisors:** Firecracker, Cloud Hypervisor
- **Container Formats:** OCI, Docker

### Performance Benchmarks

#### MicroVM Lifecycle
| Operation | Target | Notes |
|-----------|--------|-------|
| Snapshot restore | < 50ms | From memory-mapped snapshot |
| Cold boot | < 200ms | Full kernel boot |
| Network setup (Styx) | < 10ms | TAP + iptables |
| Filesystem overlay (Lethe) | < 5ms | COW setup |
| Total cold start | < 100ms | With snapshot |

#### Resource Overhead
| Metric | Per MicroVM |
|--------|-------------|
| Memory overhead | ~5MB |
| CPU overhead | ~0.1 core |
| Disk overhead | ~10MB (overlay) |
| Network overhead | 1 TAP device |

#### Scalability
| Metric | Target |
|--------|--------|
| VMs per host | 100-500 |
| Hosts per cluster | 1000+ |
| API requests/sec | 10,000+ |
| Concurrent launches | 150/sec/host |

---

## Appendix: Extended Mythology Mapping

### The Complete Pantheon

#### Primordial Deities (Foundational Components)

| Deity | Domain | System Component | Package |
|-------|--------|------------------|---------|
| **Chaos** | The Void | System Bootstrap | `cmd/` |
| **Nyx** | Night | Snapshot Manager | `pkg/nyx` |
| **Erebus** | Darkness | Deep Storage | `pkg/erebus` |
| **Tartarus** | The Pit | MicroVM Runtime | `pkg/tartarus` |

#### Titans (Heavy-Duty Components)

| Titan | Domain | System Component | Package |
|-------|--------|------------------|---------|
| **Kronos** | Time | Scheduler (Moirai) | `pkg/moirai` |
| **Hyperion** | Light | Metrics/Observability | `pkg/hermes` |
| **Prometheus** | Forethought | Predictive Scaling | `pkg/persephone` |
| **Atlas** | Endurance | Load Balancer | `pkg/charon` |

#### Olympians (Control Plane)

| God | Domain | System Component | Package |
|-----|--------|------------------|---------|
| **Zeus** | King | API Gateway | `pkg/olympus` |
| **Themis** | Law | Policy Engine | `pkg/themis` |
| **Hermes** | Messenger | Telemetry | `pkg/hermes` |

#### Chthonic Deities (Underworld)

| Deity | Domain | System Component | Package |
|-------|--------|------------------|---------|
| **Hades** | Underworld | Cluster Registry | `pkg/hades` |
| **Persephone** | Seasons | Autoscaling | `pkg/persephone` |
| **Hypnos** | Sleep | Hibernation | `pkg/hypnos` |
| **Thanatos** | Death | Termination | `pkg/thanatos` |

#### Guardians & Monsters

| Entity | Role | System Component | Package |
|--------|------|------------------|---------|
| **Hecatoncheires** | Gate Guards | Node Agents | `pkg/hecatoncheir` |
| **Cerberus** | Hound | Auth Gateway | `pkg/cerberus` |
| **Erinyes** | Avengers | Enforcement | `pkg/erinyes` |
| **Typhon** | Chaos Monster | Quarantine | `pkg/typhon` |
| **Kampe** | Old Jailor | Legacy Runtime | `pkg/kampe` |

#### Rivers of the Underworld

| River | Meaning | System Component | Package |
|-------|---------|------------------|---------|
| **Acheron** | Pain | Job Queue | `pkg/acheron` |
| **Styx** | Hatred/Oath | Network Gateway | `pkg/styx` |
| **Lethe** | Forgetting | Ephemeral FS | `pkg/lethe` |
| **Phlegethon** | Fire | Hot Path Router | `pkg/phlegethon` |
| **Cocytus** | Wailing | Error Stream | `pkg/cocytus` |

#### Judges of the Dead

| Judge | Domain | System Function |
|-------|--------|-----------------|
| **Minos** | Final Verdict | Quota validation |
| **Rhadamanthus** | Asian souls | Security policies |
| **Aeacus** | European souls | Audit/compliance |

#### The Furies (Erinyes)

| Fury | Meaning | Enforcement Type |
|------|---------|------------------|
| **Alecto** | Unceasing | Timeout enforcement |
| **Tisiphone** | Avenger | Resource limits |
| **Megaera** | Grudging | Policy violations |

#### The Fates (Moirai)

| Fate | Role | Scheduling Function |
|------|------|---------------------|
| **Clotho** | Spinner | Request creation |
| **Lachesis** | Allotter | Node selection |
| **Atropos** | Inevitable | Final assignment |

### Glossary of Tartarean Terms

| Term | Definition |
|------|------------|
| **Bronze Gates** | The network and security boundaries protecting the cluster |
| **The Pit** | The MicroVM execution environment |
| **Soul** | A sandbox request or running workload |
| **Judgment** | Admission control decision |
| **Punishment** | Enforcement action (throttle, kill, quarantine) |
| **Oath** | A network contract (Styx) |
| **Forgetting** | Ephemeral state cleanup (Lethe) |
| **Lamentation** | An error record (Cocytus) |
| **Ferry** | Request routing (Charon) |
| **Sleep** | VM hibernation (Hypnos) |
| **Death** | Graceful termination (Thanatos) |

---

## Contributing

> *"Even the gods work together. Zeus could not have defeated the Titans alone—he freed the Hecatoncheires and Cyclopes from Tartarus to join his cause."*

We welcome contributions to Tartarus! See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

### Areas of Interest

- **Performance optimization** (sub-25ms startup)
- **New runtime backends** (Cloud Hypervisor, QEMU)
- **Template development** (data science, ML, specialized environments)
- **Security hardening** (seccomp profiles, kernel patches)
- **Kubernetes integration** (CRI shim, operators)
- **Documentation and tutorials**

---

## License

Apache License 2.0

---

*"From Chaos came Erebus and black Night; but of Night were born Aether and Day."*
— Hesiod, Theogony

**Tartarus**: Secure isolation at the speed of containers.