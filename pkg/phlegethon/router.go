package phlegethon

import (
	"context"
	"time"
)

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
	NetworkBurst int // Mbps
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
	NodeSelector map[string]string // Label selector
	MinNodes     int
	MaxNodes     int
	CurrentNodes int
}

type PoolAssignment struct {
	Pool          *ResourcePool
	Node          string
	ResourceClass *ResourceClass
}

type HeatMetrics struct {
	TotalSandboxes   int
	ByHeat           map[HeatLevel]int
	QueueDepthByHeat map[HeatLevel]int
	AvgWaitByHeat    map[HeatLevel]time.Duration
}

// SandboxRequest represents a request to launch a sandbox
// This is a minimal definition to avoid circular dependencies if this package is used by others
// In a real implementation, this might be imported from a common domain package
type SandboxRequest struct {
	TemplateID  string
	HeatHint    HeatLevel
	MaxDuration time.Duration
	CPUCores    int
	MemoryMB    int
}

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

// HeatObservation records actual usage for learning
type HeatObservation struct {
	TemplateID     string
	ActualDuration time.Duration
	PeakCPU        float64
	PeakMemory     int64
	Timestamp      time.Time
}

// HeatClassifier uses heuristics to classify workloads
type HeatClassifier struct {
	// Historical data for learning
	history map[string][]HeatObservation

	// Template-based hints
	templateHints map[string]HeatLevel
}

func NewHeatClassifier() *HeatClassifier {
	return &HeatClassifier{
		history:       make(map[string][]HeatObservation),
		templateHints: make(map[string]HeatLevel),
	}
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

func (c *HeatClassifier) AddHint(templateID string, level HeatLevel) {
	c.templateHints[templateID] = level
}
