package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage sandbox snapshots",
	Long:  `Create, list, and delete snapshots for sandboxes.`,
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create [sandbox-id]",
	Short: "Create a snapshot of a running sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		path := fmt.Sprintf("/sandboxes/%s/snapshot", id)
		resp, err := doRequest(http.MethodPost, path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating snapshot: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			fmt.Fprintf(os.Stderr, "Error creating snapshot: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		fmt.Println("Snapshot creation requested")
	},
}

var snapshotListCmd = &cobra.Command{
	Use:   "list [sandbox-id]",
	Short: "List snapshots for a sandbox's template",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		path := fmt.Sprintf("/sandboxes/%s/snapshots", id)
		resp, err := doRequest(http.MethodGet, path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing snapshots: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Error listing snapshots: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		var snaps []*nyx.Snapshot
		if err := json.NewDecoder(resp.Body).Decode(&snaps); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tTEMPLATE\tCREATED\tPATH")
		for _, s := range snaps {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Template, s.CreatedAt.Format(time.RFC3339), s.Path)
		}
		w.Flush()
	},
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete [sandbox-id] [snapshot-id]",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		sandboxID := args[0]
		snapID := args[1]
		path := fmt.Sprintf("/sandboxes/%s/snapshots/%s", sandboxID, snapID)
		resp, err := doRequest(http.MethodDelete, path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting snapshot: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Error deleting snapshot: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		fmt.Println("Snapshot deleted")
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
}
