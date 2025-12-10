package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var checkpointsCmd = &cobra.Command{
	Use:   "checkpoints [sandbox-id]",
	Short: "List checkpoints for a sandbox",
	Long: `List all available checkpoints for a sandbox that can be used with 'resume'.

Examples:
  # List all checkpoints for a sandbox
  tartarus checkpoints my-sandbox

  # List all checkpoints (no sandbox filter)
  tartarus checkpoints`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sandboxID := ""
		if len(args) > 0 {
			sandboxID = args[0]
		}
		runCheckpoints(sandboxID)
	},
}

func runCheckpoints(sandboxID string) {
	path := "/sandboxes/checkpoints/"
	if sandboxID != "" {
		path = fmt.Sprintf("/sandboxes/checkpoints/%s", sandboxID)
	}

	resp, err := doRequest(http.MethodGet, path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing checkpoints: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var checkpoints []map[string]interface{}
	if err := json.Unmarshal(respBody, &checkpoints); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(checkpoints) == 0 {
		if sandboxID != "" {
			fmt.Printf("No checkpoints found for sandbox %s\n", sandboxID)
		} else {
			fmt.Println("No checkpoints found")
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSANDBOX\tTEMPLATE\tCREATED\tRESUMABLE")

	for _, cp := range checkpoints {
		id, _ := cp["id"].(string)
		sandbox, _ := cp["sandbox_id"].(string)
		template, _ := cp["template_id"].(string)
		created, _ := cp["created_at"].(string)
		resumable, _ := cp["resumable"].(bool)

		resumableStr := "no"
		if resumable {
			resumableStr = "yes"
		}

		// Truncate long IDs for display
		if len(id) > 20 {
			id = id[:17] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, sandbox, template, created, resumableStr)
	}

	w.Flush()
}

func init() {
	rootCmd.AddCommand(checkpointsCmd)
}
