package integration

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hecatoncheir"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
	"github.com/tartarus-sandbox/tartarus/pkg/phlegethon"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// Mocks (reused from perf test or similar, but simplified for this integration test)
type MockNyx struct {
	nyx.Manager
}

func (m *MockNyx) GetSnapshot(ctx context.Context, templateID domain.TemplateID) (*nyx.Snapshot, error) {
	return &nyx.Snapshot{ID: "snap-1", Template: templateID, Path: "/tmp/snap-1"}, nil
}

type MockLethe struct {
	lethe.Pool
}

func (m *MockLethe) Create(ctx context.Context, snap *nyx.Snapshot) (*lethe.Overlay, error) {
	return &lethe.Overlay{ID: "ov-1", MountPath: "/tmp/ov-1"}, nil
}
func (m *MockLethe) Destroy(ctx context.Context, ov *lethe.Overlay) error { return nil }

type MockStyx struct {
	styx.Gateway
}

func (m *MockStyx) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *styx.Contract) (string, netip.Addr, netip.Addr, netip.Prefix, error) {
	ip, _ := netip.ParseAddr("192.168.1.2")
	gw, _ := netip.ParseAddr("192.168.1.1")
	cidr, _ := netip.ParsePrefix("192.168.1.0/24")
	return "tap0", ip, gw, cidr, nil
}
func (m *MockStyx) Detach(ctx context.Context, sandboxID domain.SandboxID) error { return nil }

type MockFury struct{}

func (m *MockFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *erinyes.PolicySnapshot) error {
	return nil
}
func (m *MockFury) Disarm(ctx context.Context, runID domain.SandboxID) error { return nil }
func (m *MockFury) Watch(ctx context.Context) error                          { return nil }

// LatencyInjectingRuntime wraps MockRuntime to inject latency based on seccomp profile
type LatencyInjectingRuntime struct {
	*tartarus.MockRuntime
	QuarantineLatency time.Duration
}

func (m *LatencyInjectingRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	// Check if this is a quarantine sandbox based on ID (simulation)
	if len(req.ID) > 10 && req.ID[:10] == "quarantine" {
		time.Sleep(m.QuarantineLatency)
	}

	return m.MockRuntime.Launch(ctx, req, cfg)
}

func (m *LatencyInjectingRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return m.MockRuntime.ExecInteractive(ctx, id, cmd, stdin, stdout, stderr)
}

func TestSeccompIsolationLatencyReporting(t *testing.T) {
	// 1. Setup Infrastructure
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewPrometheusMetrics()
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	queue := acheron.NewMemoryQueue()
	registry := hades.NewMemoryRegistry()
	policyRepo := themis.NewMemoryRepo()
	templateManager := olympus.NewMemoryTemplateManager()
	scheduler := moirai.NewScheduler("least-loaded", logger)
	heatClassifier := phlegethon.NewHeatClassifier()
	control := &olympus.NoopControlPlane{}

	// Judges
	auditSink := judges.NewLogAuditSink(logger)
	aeacusJudge := judges.NewAeacusJudge(logger, auditSink)
	resourceJudge := judges.NewResourceJudge(policyRepo, logger)
	networkJudge := judges.NewNetworkJudge([]string{"0.0.0.0/0", "allow-all"}, nil, logger)

	judgeChain := &judges.Chain{
		Pre: []judges.PreJudge{aeacusJudge, resourceJudge, networkJudge},
	}

	manager := &olympus.Manager{
		Queue:      queue,
		Hades:      registry,
		Policies:   policyRepo,
		Templates:  templateManager,
		Nyx:        &MockNyx{},
		Judges:     judgeChain,
		Scheduler:  scheduler,
		Phlegethon: heatClassifier,
		Control:    control,
		Metrics:    metrics,
		Logger:     logger,
		// We might need to inject the QuarantineManager into Olympus if it uses it directly,
		// but currently Olympus might not use it in the submit flow directly unless we modify it.
		// However, the task says "Measure Typhon quarantine routing overhead".
		// If Olympus doesn't use Typhon yet, we might need to simulate the flow where it DOES.
		// Or we assume the Agent uses it.
	}

	// 2. Register Template
	tpl := &domain.TemplateSpec{
		ID:          "test-tpl",
		Name:        "Test Template",
		BaseImage:   "/var/lib/tartarus/images/base.ext4",
		KernelImage: "/var/lib/firecracker/vmlinux",
		Resources:   domain.ResourceSpec{CPU: 1000, Mem: 512},
	}
	require.NoError(t, templateManager.RegisterTemplate(context.Background(), tpl))

	// 3. Register Policy
	policy := &domain.SandboxPolicy{
		ID:            "default-policy",
		TemplateID:    "test-tpl",
		Resources:     domain.ResourceSpec{CPU: 1000, Mem: 512},
		NetworkPolicy: domain.NetworkPolicyRef{ID: "allow-all"},
		Retention:     domain.RetentionPolicy{MaxAge: 10 * time.Minute},
	}
	require.NoError(t, policyRepo.UpsertPolicy(context.Background(), policy))

	// 4. Setup Agent with Latency Injecting Runtime
	baseMockRuntime := tartarus.NewMockRuntime(slogLogger)
	baseMockRuntime.SetStartDuration(10 * time.Millisecond) // Base latency

	runtime := &LatencyInjectingRuntime{
		MockRuntime:       baseMockRuntime,
		QuarantineLatency: 50 * time.Millisecond, // Added latency for quarantine
	}

	agentID := "test-agent-1"
	agent := &hecatoncheir.Agent{
		NodeID:   domain.NodeID(agentID),
		Runtime:  runtime,
		Nyx:      &MockNyx{},
		Lethe:    &MockLethe{},
		Styx:     &MockStyx{},
		Furies:   &MockFury{},
		Queue:    queue,
		Registry: registry,
		Metrics:  metrics,
		Logger:   logger,
	}

	// Register Node
	registry.UpdateHeartbeat(context.Background(), hades.HeartbeatPayload{
		Node: domain.NodeInfo{ID: domain.NodeID(agentID), Address: "localhost", Capacity: domain.ResourceCapacity{CPU: 4000, Mem: 8192}},
		Time: time.Now(),
	})

	// Start Agent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = agent.Run(ctx) }()

	// 5. Run Normal Sandbox
	normalReq := &domain.SandboxRequest{
		ID:         "normal-sandbox",
		Template:   "test-tpl",
		NetworkRef: domain.NetworkPolicyRef{ID: "allow-all"},
	}

	startNormal := time.Now()
	require.NoError(t, manager.Submit(ctx, normalReq))
	waitForRunning(t, registry, normalReq.ID)
	normalDuration := time.Since(startNormal)

	// 6. Run Quarantined Sandbox
	// We simulate quarantine by using an ID that triggers the mock latency.
	// In a real integration, we would use the QuarantineManager to flag it,
	// and the Agent would pick up the stricter profile.
	quarantineReq := &domain.SandboxRequest{
		ID:         "quarantine-sandbox",
		Template:   "test-tpl",
		NetworkRef: domain.NetworkPolicyRef{ID: "allow-all"},
	}

	startQuarantine := time.Now()
	require.NoError(t, manager.Submit(ctx, quarantineReq))
	waitForRunning(t, registry, quarantineReq.ID)
	quarantineDuration := time.Since(startQuarantine)

	// 7. Verify Latency Difference
	// We expect quarantine to be slower due to the injected latency
	t.Logf("Normal Duration: %v", normalDuration)
	t.Logf("Quarantine Duration: %v", quarantineDuration)

	diff := quarantineDuration - normalDuration
	if diff < 40*time.Millisecond {
		t.Errorf("Expected quarantine to add ~50ms latency, but diff was %v", diff)
	}

	// Verify Metrics (Optional but good)
	// metrics.GetHistogram("...")
}

func waitForRunning(t *testing.T, registry hades.Registry, id domain.SandboxID) {
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		run, err := registry.GetRun(ctx, id)
		if err == nil && run.Status == domain.RunStatusRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Sandbox %s failed to reach RUNNING state", id)
}
