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

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List active sandboxes",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := http.Get(host + "/sandboxes")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing sandboxes: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Request failed with status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		var runs []domain.SandboxRun
		if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tTEMPLATE\tSTATUS\tSTARTED\tNODE")
		for _, run := range runs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				run.ID,
				run.Template,
				run.Status,
				time.Since(run.StartedAt).Round(time.Second),
				run.NodeID,
			)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(psCmd)
}
