package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill [id]",
	Short: "Terminate a sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		url := fmt.Sprintf("%s/sandboxes/%s", host, id)

		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
			os.Exit(1)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error terminating sandbox: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		fmt.Printf("Sandbox %s terminated\n", id)
	},
}

func init() {
	rootCmd.AddCommand(killCmd)
}
