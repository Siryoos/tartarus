package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var resumeNode string

var resumeCmd = &cobra.Command{
	Use:   "resume [checkpoint-id]",
	Short: "Resume a sandbox from checkpoint",
	Long: `Resume a previously checkpointed sandbox.

Examples:
  # Resume from checkpoint
  tartarus resume checkpoint-abc123

  # Resume on a specific node
  tartarus resume checkpoint-abc123 --node node-1`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		checkpointID := args[0]
		runResume(checkpointID)
	},
}

func runResume(checkpointID string) {
	body := map[string]interface{}{
		"checkpoint_id": checkpointID,
	}
	if resumeNode != "" {
		body["override_node"] = resumeNode
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := doRequest(http.MethodPost, "/sandboxes/resume", bytes.NewReader(jsonBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resuming sandbox: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "Checkpoint not found: %s\n", checkpointID)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("Sandbox resumed from checkpoint %s\n", checkpointID)
		return
	}

	sandboxID, _ := result["sandbox_id"].(string)
	status, _ := result["status"].(string)
	resumedFrom, _ := result["resumed_from"].(string)

	fmt.Printf("Sandbox ID: %s\n", sandboxID)
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Resumed From: %s\n", resumedFrom)
}

func init() {
	resumeCmd.Flags().StringVarP(&resumeNode, "node", "n", "", "Override target node for resume")

	rootCmd.AddCommand(resumeCmd)
}
