package tartarus

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// MockRuntimeWithDelay simulates a runtime with configurable latency
type MockRuntimeWithDelay struct {
	MockRuntime
	LaunchDelay time.Duration
	ExecDelay   time.Duration
}

func (m *MockRuntimeWithDelay) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	time.Sleep(m.LaunchDelay)
	return &domain.SandboxRun{
		ID:       req.ID,
		Status:   domain.RunStatusRunning,
		Metadata: make(map[string]string),
	}, nil
}

func (m *MockRuntimeWithDelay) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	time.Sleep(m.ExecDelay)
	return nil
}

func BenchmarkUnifiedRuntime_Launch(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := hermes.NewPrometheusMetrics()

	// Simulate MicroVM (slow launch)
	microVM := &MockRuntimeWithDelay{LaunchDelay: 100 * time.Millisecond}
	// Simulate WASM (fast launch)
	wasm := &MockRuntimeWithDelay{LaunchDelay: 5 * time.Millisecond}
	// Simulate gVisor (medium launch)
	gvisor := &MockRuntimeWithDelay{LaunchDelay: 50 * time.Millisecond}

	rt := NewUnifiedRuntime(UnifiedRuntimeConfig{
		MicroVMRuntime: microVM,
		WasmRuntime:    wasm,
		GVisorRuntime:  gvisor,
		DefaultRuntime: IsolationMicroVM,
		AutoSelect:     true,
		Logger:         logger,
		Metrics:        metrics,
	})

	ctx := context.Background()

	b.Run("MicroVM_Launch", func(b *testing.B) {
		req := &domain.SandboxRequest{
			ID: "bench-microvm",
			Resources: domain.ResourceSpec{
				CPU: 4000, // Forces MicroVM
				Mem: 8192,
			},
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req.ID = domain.SandboxID("bench-microvm-" + string(rune(i)))
			_, _ = rt.Launch(ctx, req, VMConfig{})
		}
	})

	b.Run("WASM_Launch", func(b *testing.B) {
		req := &domain.SandboxRequest{
			ID: "bench-wasm",
			Resources: domain.ResourceSpec{
				CPU: 100, // Forces WASM
				Mem: 64,
			},
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req.ID = domain.SandboxID("bench-wasm-" + string(rune(i)))
			_, _ = rt.Launch(ctx, req, VMConfig{})
		}
	})

	b.Run("gVisor_Launch_Explicit", func(b *testing.B) {
		req := &domain.SandboxRequest{
			ID: "bench-gvisor",
			Metadata: map[string]string{
				"isolation_type": "gvisor",
			},
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req.ID = domain.SandboxID("bench-gvisor-" + string(rune(i)))
			_, _ = rt.Launch(ctx, req, VMConfig{})
		}
	})
}
