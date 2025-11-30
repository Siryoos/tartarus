package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
)

// Mock Server
func startMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Logs
	mux.HandleFunc("/sandboxes/logs/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/sandboxes/logs/"):]
		follow := r.URL.Query().Get("follow")
		if id == "test-id" {
			fmt.Fprintf(w, "log line 1\n")
			if follow == "true" {
				fmt.Fprintf(w, "log line 2 (followed)\n")
			}
		} else {
			http.NotFound(w, r)
		}
	})

	// Snapshots
	mux.HandleFunc("/sandboxes/test-id/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
		}
	})
	mux.HandleFunc("/sandboxes/test-id/snapshots", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]*nyx.Snapshot{
				{ID: "snap-1", Template: "tpl-1", Path: "/tmp/snap-1"},
			})
		}
	})
	mux.HandleFunc("/sandboxes/test-id/snapshots/snap-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
		}
	})

	// Exec
	mux.HandleFunc("/sandboxes/test-id/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
		}
	})

	// Inspect
	mux.HandleFunc("/sandboxes/test-id", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(domain.SandboxRun{
				ID:       "test-id",
				Status:   domain.RunStatusRunning,
				Template: "test-tpl",
			})
		}
	})

	return httptest.NewServer(mux)
}

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestInitTemplate(t *testing.T) {
	// Create a dummy Dockerfile
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	content := `
FROM alpine:3.14
ENV FOO=bar
WORKDIR /app
`
	err := os.WriteFile(dockerfile, []byte(content), 0644)
	require.NoError(t, err)

	// Run init template
	// We need to change directory or specify path.
	// The command writes to current directory.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(tmpDir)

	output, err := executeCommand(rootCmd, "init", "template", "my-tpl", "--dockerfile", dockerfile)
	require.NoError(t, err)
	assert.Contains(t, output, "Template scaffolded to my-tpl.yaml")

	// Verify file content
	yamlContent, err := os.ReadFile(filepath.Join(tmpDir, "my-tpl.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(yamlContent), "base_image: alpine:3.14")
	assert.Contains(t, string(yamlContent), "FOO: bar")
	assert.Contains(t, string(yamlContent), "working_dir: /app")
}

func TestLogs(t *testing.T) {
	server := startMockServer(t)
	defer server.Close()

	host = server.URL

	// Test Logs
	output, err := executeCommand(rootCmd, "logs", "test-id")
	require.NoError(t, err)
	assert.Contains(t, output, "log line 1")

	// Test Logs Follow
	output, err = executeCommand(rootCmd, "logs", "test-id", "--follow")
	require.NoError(t, err)
	assert.Contains(t, output, "log line 1")
	assert.Contains(t, output, "log line 2 (followed)")
}

func TestSnapshot(t *testing.T) {
	server := startMockServer(t)
	defer server.Close()
	host = server.URL

	// Create
	output, err := executeCommand(rootCmd, "snapshot", "create", "test-id")
	require.NoError(t, err)
	assert.Contains(t, output, "Snapshot creation requested")

	// List
	output, err = executeCommand(rootCmd, "snapshot", "list", "test-id")
	require.NoError(t, err)
	assert.Contains(t, output, "snap-1")
	assert.Contains(t, output, "tpl-1")

	// Delete
	output, err = executeCommand(rootCmd, "snapshot", "delete", "test-id", "snap-1")
	require.NoError(t, err)
	assert.Contains(t, output, "Snapshot deleted")
}

func TestExec(t *testing.T) {
	server := startMockServer(t)
	defer server.Close()
	host = server.URL

	output, err := executeCommand(rootCmd, "exec", "test-id", "--", "ls", "-la")
	require.NoError(t, err)
	assert.Contains(t, output, "Exec command requested")
}

func TestInspect(t *testing.T) {
	server := startMockServer(t)
	defer server.Close()
	host = server.URL

	output, err := executeCommand(rootCmd, "inspect", "test-id")
	require.NoError(t, err)
	assert.Contains(t, output, "ID:")
	assert.Contains(t, output, "test-id")
	assert.Contains(t, output, "Status:")
	assert.Contains(t, output, "RUNNING")
}
