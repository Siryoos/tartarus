package kampe

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// ParityTestCase defines a test case for runtime parity verification
type ParityTestCase struct {
	Name    string
	Request *domain.SandboxRequest
	Config  tartarus.VMConfig
	ExpectedBehavior
	SkipRuntimes []string // Runtimes to skip for this test
}

// ExpectedBehavior defines what we expect from a runtime execution
type ExpectedBehavior struct {
	ExitCode       *int
	StdoutContains []string
	StderrContains []string
	MaxDuration    time.Duration
}

// RuntimeResult captures the result of running a test on one runtime
type RuntimeResult struct {
	RuntimeName string
	Run         *domain.SandboxRun
	Stdout      string
	Stderr      string
	Duration    time.Duration
	Error       error
}

// ParityHarness runs the same workload across different runtimes and compares results
type ParityHarness struct {
	Runtimes map[string]tartarus.SandboxRuntime
	Timeout  time.Duration
}

// NewParityHarness creates a new parity test harness
func NewParityHarness() *ParityHarness {
	return &ParityHarness{
		Runtimes: make(map[string]tartarus.SandboxRuntime),
		Timeout:  60 * time.Second,
	}
}

// AddRuntime registers a runtime for parity testing
func (h *ParityHarness) AddRuntime(name string, runtime tartarus.SandboxRuntime) {
	h.Runtimes[name] = runtime
}

// RunTest executes a test case against all registered runtimes
func (h *ParityHarness) RunTest(ctx context.Context, tc ParityTestCase) (map[string]*RuntimeResult, error) {
	results := make(map[string]*RuntimeResult)

	for name, runtime := range h.Runtimes {
		// Check if this runtime should be skipped
		skip := false
		for _, skipName := range tc.SkipRuntimes {
			if skipName == name {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		result := h.runOnRuntime(ctx, name, runtime, tc)
		results[name] = result
	}

	return results, nil
}

// runOnRuntime executes the test case on a single runtime
func (h *ParityHarness) runOnRuntime(ctx context.Context, name string, runtime tartarus.SandboxRuntime, tc ParityTestCase) *RuntimeResult {
	result := &RuntimeResult{
		RuntimeName: name,
	}

	// Create a unique ID for this run
	req := *tc.Request
	req.ID = domain.SandboxID(fmt.Sprintf("%s-%s-%d", tc.Name, name, time.Now().UnixNano()))

	testCtx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()

	start := time.Now()

	// Launch the sandbox
	run, err := runtime.Launch(testCtx, &req, tc.Config)
	if err != nil {
		result.Error = fmt.Errorf("launch failed: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	result.Run = run

	// Wait for completion
	if err := runtime.Wait(testCtx, req.ID); err != nil {
		result.Error = fmt.Errorf("wait failed: %w", err)
	}

	// Collect logs
	var stdoutBuf bytes.Buffer
	if err := runtime.StreamLogs(testCtx, req.ID, &stdoutBuf, false); err == nil {
		result.Stdout = stdoutBuf.String()
	}

	// Get final status
	finalRun, err := runtime.Inspect(testCtx, req.ID)
	if err == nil {
		result.Run = finalRun
	}

	result.Duration = time.Since(start)

	// Cleanup
	_ = runtime.Kill(context.Background(), req.ID)

	return result
}

// Compare compares results across runtimes and reports differences
func (h *ParityHarness) Compare(t *testing.T, results map[string]*RuntimeResult, expected ExpectedBehavior) {
	t.Helper()

	// Collect all runtimes that succeeded
	var successfulResults []*RuntimeResult
	for _, result := range results {
		if result.Error == nil {
			successfulResults = append(successfulResults, result)
		} else {
			t.Logf("Runtime %s failed: %v", result.RuntimeName, result.Error)
		}
	}

	if len(successfulResults) < 2 {
		t.Skip("Not enough runtimes succeeded for comparison")
	}

	// Compare exit codes
	if expected.ExitCode != nil {
		for _, result := range successfulResults {
			if result.Run.ExitCode == nil {
				t.Errorf("Runtime %s: expected exit code %d, got nil", result.RuntimeName, *expected.ExitCode)
			} else if *result.Run.ExitCode != *expected.ExitCode {
				t.Errorf("Runtime %s: expected exit code %d, got %d", result.RuntimeName, *expected.ExitCode, *result.Run.ExitCode)
			}
		}
	}

	// Compare exit codes across runtimes (parity check)
	refResult := successfulResults[0]
	for _, result := range successfulResults[1:] {
		if !equalExitCodes(refResult.Run.ExitCode, result.Run.ExitCode) {
			t.Errorf("Exit code mismatch: %s=%v, %s=%v",
				refResult.RuntimeName, refResult.Run.ExitCode,
				result.RuntimeName, result.Run.ExitCode)
		}
	}

	// Check expected stdout content
	for _, contains := range expected.StdoutContains {
		for _, result := range successfulResults {
			if !strings.Contains(result.Stdout, contains) {
				t.Errorf("Runtime %s: stdout missing expected content %q", result.RuntimeName, contains)
			}
		}
	}

	// Check expected stderr content
	for _, contains := range expected.StderrContains {
		for _, result := range successfulResults {
			if !strings.Contains(result.Stderr, contains) {
				t.Errorf("Runtime %s: stderr missing expected content %q", result.RuntimeName, contains)
			}
		}
	}

	// Check duration limit
	if expected.MaxDuration > 0 {
		for _, result := range successfulResults {
			if result.Duration > expected.MaxDuration {
				t.Errorf("Runtime %s: duration %v exceeds max %v", result.RuntimeName, result.Duration, expected.MaxDuration)
			}
		}
	}
}

func equalExitCodes(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// StandardParityTests returns a list of standard parity test cases
func StandardParityTests() []ParityTestCase {
	exitCodeZero := 0
	exitCode42 := 42

	return []ParityTestCase{
		{
			Name: "basic-exit-0",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"true"},
				Args:     []string{},
				Env:      map[string]string{},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 128,
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:    &exitCodeZero,
				MaxDuration: 30 * time.Second,
			},
		},
		{
			Name: "custom-exit-code",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"sh", "-c", "exit 42"},
				Args:     []string{},
				Env:      map[string]string{},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 128,
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:    &exitCode42,
				MaxDuration: 30 * time.Second,
			},
		},
		{
			Name: "echo-stdout",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"echo", "hello-parity-test"},
				Args:     []string{},
				Env:      map[string]string{},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 128,
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:       &exitCodeZero,
				StdoutContains: []string{"hello-parity-test"},
				MaxDuration:    30 * time.Second,
			},
		},
		{
			Name: "environment-variables",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"sh", "-c", "echo $TEST_VAR"},
				Args:     []string{},
				Env: map[string]string{
					"TEST_VAR": "parity-env-value",
				},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 128,
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:       &exitCodeZero,
				StdoutContains: []string{"parity-env-value"},
				MaxDuration:    30 * time.Second,
			},
		},
		{
			Name: "working-directory",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"pwd"},
				Args:     []string{},
				Env:      map[string]string{},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 128,
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:       &exitCodeZero,
				StdoutContains: []string{"/"},
				MaxDuration:    30 * time.Second,
			},
		},
		{
			Name: "resource-limits",
			Request: &domain.SandboxRequest{
				Template: "alpine:latest",
				Command:  []string{"sh", "-c", "cat /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null || cat /sys/fs/cgroup/memory.max 2>/dev/null || echo 'no-cgroup'"},
				Args:     []string{},
				Env:      map[string]string{},
				Resources: domain.ResourceSpec{
					CPU: 1000,
					Mem: 256, // 256 MB limit
				},
			},
			Config: tartarus.VMConfig{},
			ExpectedBehavior: ExpectedBehavior{
				ExitCode:    &exitCodeZero,
				MaxDuration: 30 * time.Second,
			},
		},
	}
}

// BenchmarkParity runs performance comparison across runtimes
func BenchmarkParity(b *testing.B, harness *ParityHarness, tc ParityTestCase) {
	ctx := context.Background()

	for name := range harness.Runtimes {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				results, _ := harness.RunTest(ctx, tc)
				if result, ok := results[name]; ok && result.Error != nil {
					b.Errorf("Runtime %s failed: %v", name, result.Error)
				}
			}
		})
	}
}
