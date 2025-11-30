package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [sandbox-id] [command...]",
	Short: "Execute a command in a running sandbox",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		command := args[1:]

		reqBody := struct {
			Cmd []string `json:"cmd"`
		}{
			Cmd: command,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling request: %v\n", err)
			os.Exit(1)
		}

		path := fmt.Sprintf("/sandboxes/%s/exec", id)
		resp, err := doRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			fmt.Fprintf(os.Stderr, "Error executing command: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		fmt.Println("Exec command requested")
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
