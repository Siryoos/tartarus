package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestOlympusPersistence(t *testing.T) {
	// 1. Start Miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// 2. Build Olympus API
	// We need to build the binary to run it as a separate process
	// Assuming we are running from the root of the repo
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Adjust wd if we are running from tests/integration
	if filepath.Base(wd) == "integration" {
		wd = filepath.Dir(filepath.Dir(wd))
	} else if filepath.Base(wd) == "tests" {
		wd = filepath.Dir(wd)
	}

	binaryPath := filepath.Join(wd, "bin", "olympus-api-test")
	cmdBuild := exec.Command("go", "build", "-o", binaryPath, "./cmd/olympus-api")
	cmdBuild.Dir = wd
	out, err := cmdBuild.CombinedOutput()
	require.NoError(t, err, "Failed to build olympus-api: %s", string(out))
	defer os.Remove(binaryPath)

	// Helper to find a free port
	getFreePort := func() int {
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		require.NoError(t, err)
		l, err := net.ListenTCP("tcp", addr)
		require.NoError(t, err)
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	port := getFreePort()

	// 3. Helper to start Olympus
	startOlympus := func() *exec.Cmd {
		cmd := exec.Command(binaryPath)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("REDIS_ADDR=%s", mr.Addr()),
			fmt.Sprintf("PORT=%d", port),
			"LOG_LEVEL=ERROR",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Start()
		require.NoError(t, err)
		return cmd
	}

	// 4. Start Olympus (First Run)
	serverCmd := startOlympus()
	// Ensure cleanup if test fails
	defer func() {
		if serverCmd.Process != nil {
			serverCmd.Process.Kill()
		}
	}()

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Wait for server to be ready
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/sandboxes")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "Server failed to start")

	// 4.5. Register a node in Redis
	node := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID:      "test-node-1",
			Address: "127.0.0.1",
			Capacity: domain.ResourceCapacity{
				CPU: 8000,
				Mem: 8192,
			},
		},
		Heartbeat: time.Now(),
	}
	nodeData, _ := json.Marshal(node)
	mr.Set("tartarus:node:test-node-1", string(nodeData))

	// 5. Submit a job
	req := domain.SandboxRequest{
		ID:       "test-job-1",
		Template: "hello-world",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
		NetworkRef: domain.NetworkPolicyRef{
			ID: "lockdown-no-net",
		},
	}
	reqBody, _ := json.Marshal(req)
	resp, err := http.Post(baseURL+"/submit", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	// 6. Verify job exists
	resp, err = http.Get(baseURL + "/sandboxes")
	require.NoError(t, err)
	var runs []domain.SandboxRun
	json.NewDecoder(resp.Body).Decode(&runs)
	resp.Body.Close()

	found := false
	for _, r := range runs {
		if r.ID == "test-job-1" {
			found = true
			break
		}
	}
	assert.True(t, found, "Job should be found in first run")

	// 7. Kill Olympus
	err = serverCmd.Process.Kill()
	require.NoError(t, err)
	serverCmd.Wait()

	// 8. Restart Olympus (Second Run)
	// Redis (mr) is still running and holding data
	serverCmd = startOlympus()
	// Defer is already set up to kill serverCmd, but we updated the variable.
	// The defer captures the variable by reference? No, by value if not pointer?
	// Wait, serverCmd is a pointer to exec.Cmd.
	// But if I assign a new pointer to serverCmd, the defer might still point to the old one if it closed over the value.
	// Actually, I should use a wrapper or update the cleanup logic.
	// Or just add another defer for the second process.
	defer func() {
		if serverCmd.Process != nil {
			serverCmd.Process.Kill()
		}
	}()

	// Wait for server to be ready
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/sandboxes")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "Server failed to restart")

	// 9. Verify job still exists
	resp, err = http.Get(baseURL + "/sandboxes")
	require.NoError(t, err)
	var runs2 []domain.SandboxRun
	json.NewDecoder(resp.Body).Decode(&runs2)
	resp.Body.Close()

	found = false
	for _, r := range runs2 {
		if r.ID == "test-job-1" {
			found = true
			break
		}
	}
	assert.True(t, found, "Job should persist across restarts")

	// 10. Verify Policy Persistence
	// Since we can't easily create a policy via API (no POST /policies), we'll verify the default policy exists and has correct version.
	// In a real scenario, we'd want to modify it and check persistence, but without an API, we rely on the repo implementation tests
	// and this integration test proving the repo is wired correctly (i.e. it doesn't crash and returns data).

	resp, err = http.Get(baseURL + "/policies")
	require.NoError(t, err)
	var policies []domain.SandboxPolicy
	json.NewDecoder(resp.Body).Decode(&policies)
	resp.Body.Close()

	assert.NotEmpty(t, policies, "Should have at least the default policy")
	foundDefault := false
	for _, p := range policies {
		if p.ID == "default-hello-world" {
			foundDefault = true
			break
		}
	}
	assert.True(t, foundDefault, "Default policy should be present after restart")
}
