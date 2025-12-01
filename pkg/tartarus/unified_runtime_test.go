package tartarus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestWasmRuntime_Launch(t *testing.T) {
	// Create temp work directory
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rt := NewWasmRuntime(logger, tmpDir)

	// Create a simple WASM module file (stub for testing)
	wasmDir := filepath.Join(tmpDir, "wasm")
	if err := os.MkdirAll(wasmDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Note: In real tests, you would use an actual .wasm file
	// For this test, we'll just verify the structure works
	ctx := context.Background()

	req := &domain.SandboxRequest{
		ID:       "test-wasm-1",
		Template: "wasm-test",
		NodeID:   "node-1",
		Command:  []string{"test.wasm"},
		Args:     []string{"arg1", "arg2"},
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
		Resources: domain.ResourceSpec{
			CPU: 100,
			Mem: 64,
		},
		Metadata: map[string]string{
			"isolation_type": "wasm",
		},
		CreatedAt: time.Now(),
	}

	cfg := VMConfig{
		Snapshot: domain.SnapshotRef{
			Path: filepath.Join(wasmDir, "test.wasm"),
		},
	}

	// This will fail because we don't have an actual WASM file,
	// but it tests the structure
	_, err := rt.Launch(ctx, req, cfg)
	if err == nil {
		t.Error("Expected error for missing WASM file")
	}

	// Test List (should be empty after failure)
	runs, err := rt.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("Expected 0 runs, got %d", len(runs))
	}
}

func TestUnifiedRuntime_AutoSelection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock runtimes
	mockMicroVM := NewMockRuntime(logger)
	mockWasm := NewMockRuntime(logger)

	rt := NewUnifiedRuntime(UnifiedRuntimeConfig{
		MicroVMRuntime: mockMicroVM,
		WasmRuntime:    mockWasm,
		DefaultRuntime: IsolationMicroVM,
		AutoSelect:     true,
		Logger:         logger,
	})

	ctx := context.Background()

	// Test 1: Lightweight workload should select WASM
	lightReq := &domain.SandboxRequest{
		ID:       "light-1",
		Template: "test",
		NodeID:   "node-1",
		Resources: domain.ResourceSpec{
			CPU: 100, // 0.1 core
			Mem: 64,  // 64MB
			TTL: 30 * time.Second,
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now(),
	}

	// Test 2: Heavy workload should select MicroVM
	heavyReq := &domain.SandboxRequest{
		ID:       "heavy-1",
		Template: "test",
		NodeID:   "node-1",
		Resources: domain.ResourceSpec{
			CPU: 4000, // 4 cores
			Mem: 8192, // 8GB
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now(),
	}

	// Test 3: GPU workload should select MicroVM
	gpuReq := &domain.SandboxRequest{
		ID:       "gpu-1",
		Template: "test",
		NodeID:   "node-1",
		Resources: domain.ResourceSpec{
			CPU: 2000,
			Mem: 4096,
			GPU: domain.GPURequest{Count: 1, Type: "nvidia-t4"},
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now(),
	}

	// Test runtime selection
	selector := NewRuntimeSelector(logger)

	lightType := selector.SelectRuntime(lightReq)
	if lightType != IsolationWASM {
		t.Errorf("Expected WASM for lightweight workload, got %s", lightType)
	}

	heavyType := selector.SelectRuntime(heavyReq)
	if heavyType != IsolationMicroVM {
		t.Errorf("Expected MicroVM for heavy workload, got %s", heavyType)
	}

	gpuType := selector.SelectRuntime(gpuReq)
	if gpuType != IsolationMicroVM {
		t.Errorf("Expected MicroVM for GPU workload, got %s", gpuType)
	}

	// Test explicit runtime type in metadata
	explicitReq := &domain.SandboxRequest{
		ID:       "explicit-1",
		Template: "test",
		NodeID:   "node-1",
		Resources: domain.ResourceSpec{
			CPU: 100,
			Mem: 64,
		},
		Metadata: map[string]string{
			"isolation_type": "microvm",
		},
		CreatedAt: time.Now(),
	}

	cfg := VMConfig{}

	run, err := rt.Launch(ctx, explicitReq, cfg)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	if run.Metadata["runtime_type"] != "microvm" {
		t.Errorf("Expected runtime_type=microvm, got %s", run.Metadata["runtime_type"])
	}

	// Test List aggregation
	runs, err := rt.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(runs) == 0 {
		t.Error("Expected at least 1 run from List")
	}

	// All runs should have runtime_type metadata
	for _, run := range runs {
		if run.Metadata["runtime_type"] == "" {
			t.Error("Run missing runtime_type metadata")
		}
	}
}

func TestUnifiedRuntime_Delegation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mockMicroVM := NewMockRuntime(logger)

	rt := NewUnifiedRuntime(UnifiedRuntimeConfig{
		MicroVMRuntime: mockMicroVM,
		DefaultRuntime: IsolationMicroVM,
		AutoSelect:     false,
		Logger:         logger,
	})

	ctx := context.Background()

	req := &domain.SandboxRequest{
		ID:        "test-1",
		Template:  "test",
		NodeID:    "node-1",
		Resources: domain.ResourceSpec{CPU: 1000, Mem: 512},
		Metadata:  map[string]string{},
		CreatedAt: time.Now(),
	}

	cfg := VMConfig{}

	// Launch sandbox
	run, err := rt.Launch(ctx, req, cfg)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	// Test Inspect
	inspected, err := rt.Inspect(ctx, run.ID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if inspected.ID != run.ID {
		t.Errorf("Expected ID %s, got %s", run.ID, inspected.ID)
	}

	// Test Kill
	err = rt.Kill(ctx, run.ID)
	if err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	// Test Allocation
	alloc, err := rt.Allocation(ctx)
	if err != nil {
		t.Fatalf("Allocation failed: %v", err)
	}
	if alloc.CPU < 0 || alloc.Mem < 0 {
		t.Error("Allocation returned negative values")
	}
}
