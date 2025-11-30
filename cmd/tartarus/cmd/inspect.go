package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [sandbox-id]",
	Short: "Inspect a sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		path := fmt.Sprintf("/sandboxes/%s", id)
		resp, err := doRequest(http.MethodGet, path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error inspecting sandbox: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Error inspecting sandbox: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		var run domain.SandboxRun
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintf(w, "ID:\t%s\n", run.ID)
		fmt.Fprintf(w, "Template:\t%s\n", run.Template)
		fmt.Fprintf(w, "Status:\t%s\n", run.Status)
		fmt.Fprintf(w, "Node:\t%s\n", run.NodeID)
		fmt.Fprintf(w, "Created:\t%s\n", run.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "Updated:\t%s\n", run.UpdatedAt.Format(time.RFC3339))
		if run.Error != "" {
			fmt.Fprintf(w, "Error:\t%s\n", run.Error)
		}
		if len(run.Metadata) > 0 {
			fmt.Fprintln(w, "Metadata:")
			for k, v := range run.Metadata {
				fmt.Fprintf(w, "  %s:\t%s\n", k, v)
			}
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}
