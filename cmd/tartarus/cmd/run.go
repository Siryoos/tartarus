package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

var (
	runCmdStr string
	runMem    int
)

var runCmd = &cobra.Command{
	Use:   "run [image]",
	Short: "Submit a job to Olympus",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		image := args[0]

		// Construct request
		req := domain.SandboxRequest{
			ID:       domain.SandboxID(fmt.Sprintf("req-%d", time.Now().UnixNano())), // Client-side ID generation for now
			Template: domain.TemplateID(image),                                       // Using image as template ID for now
			Command:  []string{"/bin/sh", "-c", runCmdStr},
			Resources: domain.ResourceSpec{
				CPU: 1000, // Default 1 CPU
				Mem: domain.Megabytes(runMem),
			},
			CreatedAt: time.Now(),
		}

		body, err := json.Marshal(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling request: %v\n", err)
			os.Exit(1)
		}

		resp, err := doRequest(http.MethodPost, "/submit", bytes.NewBuffer(body))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error submitting request: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(result["id"])
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&runCmdStr, "cmd", "", "Command to run")
	runCmd.Flags().IntVar(&runMem, "mem", 128, "Memory in MB")
	runCmd.MarkFlagRequired("cmd")
}
