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

var (
	terminateDelay      string
	terminateGrace      string
	terminateCheckpoint bool
	terminateReason     string
)

var terminateCmd = &cobra.Command{
	Use:   "terminate [sandbox-id]",
	Short: "Gracefully terminate a sandbox",
	Long: `Gracefully terminate a sandbox with optional delay and checkpoint creation.

Examples:
  # Immediate graceful termination
  tartarus terminate my-sandbox

  # Deferred termination (after 5 minutes)
  tartarus terminate my-sandbox --delay 5m

  # Terminate with checkpoint
  tartarus terminate my-sandbox --checkpoint

  # Custom grace period
  tartarus terminate my-sandbox --grace 30s`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		runTerminate(id)
	},
}

func runTerminate(id string) {
	body := map[string]interface{}{}
	if terminateDelay != "" {
		body["delay"] = terminateDelay
	}
	if terminateGrace != "" {
		body["grace_period"] = terminateGrace
	}
	if terminateCheckpoint {
		body["create_checkpoint"] = true
	}
	if terminateReason != "" {
		body["reason"] = terminateReason
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := doRequest(http.MethodPost, fmt.Sprintf("/sandboxes/terminate/%s", id), bytes.NewReader(jsonBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scheduling termination: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("Termination scheduled for sandbox %s\n", id)
		return
	}

	status, _ := result["status"].(string)
	terminationID, _ := result["termination_id"].(string)
	scheduledAt, _ := result["scheduled_at"].(string)

	fmt.Printf("Sandbox: %s\n", id)
	fmt.Printf("Status: %s\n", status)
	if terminationID != "" {
		fmt.Printf("Termination ID: %s\n", terminationID)
	}
	if scheduledAt != "" {
		fmt.Printf("Scheduled At: %s\n", scheduledAt)
	}
	if terminateCheckpoint {
		fmt.Println("Checkpoint will be created before termination")
	}
}

var terminateStatusCmd = &cobra.Command{
	Use:   "status [sandbox-id]",
	Short: "Get termination status for a sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]

		resp, err := doRequest(http.MethodGet, fmt.Sprintf("/sandboxes/terminate/%s", id), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting termination status: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("No termination found for sandbox %s\n", id)
			return
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
			os.Exit(1)
		}

		status, _ := result["status"].(string)
		terminationID, _ := result["termination_id"].(string)
		scheduledAt, _ := result["scheduled_at"].(string)
		checkpointID, _ := result["checkpoint_id"].(string)

		fmt.Printf("Sandbox: %s\n", id)
		fmt.Printf("Status: %s\n", status)
		if terminationID != "" {
			fmt.Printf("Termination ID: %s\n", terminationID)
		}
		if scheduledAt != "" {
			fmt.Printf("Scheduled At: %s\n", scheduledAt)
		}
		if checkpointID != "" {
			fmt.Printf("Checkpoint ID: %s\n", checkpointID)
		}
	},
}

var terminateCancelCmd = &cobra.Command{
	Use:   "cancel [sandbox-id]",
	Short: "Cancel a pending termination",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]

		resp, err := doRequest(http.MethodDelete, fmt.Sprintf("/sandboxes/terminate/%s", id), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error cancelling termination: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("No termination found for sandbox %s\n", id)
			os.Exit(1)
		}

		if resp.StatusCode == http.StatusConflict {
			fmt.Printf("Termination already cancelled for sandbox %s\n", id)
			os.Exit(1)
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Printf("Termination cancelled for sandbox %s\n", id)
	},
}

func init() {
	terminateCmd.Flags().StringVarP(&terminateDelay, "delay", "d", "", "Delay before termination starts (e.g., 5m, 30s)")
	terminateCmd.Flags().StringVarP(&terminateGrace, "grace", "g", "", "Grace period for shutdown (e.g., 10s, 1m)")
	terminateCmd.Flags().BoolVarP(&terminateCheckpoint, "checkpoint", "c", false, "Create checkpoint before termination")
	terminateCmd.Flags().StringVarP(&terminateReason, "reason", "r", "", "Reason for termination")

	terminateCmd.AddCommand(terminateStatusCmd)
	terminateCmd.AddCommand(terminateCancelCmd)

	rootCmd.AddCommand(terminateCmd)
}
