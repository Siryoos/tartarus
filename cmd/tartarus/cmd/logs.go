package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [id]",
	Short: "Stream logs from a sandbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		follow, _ := cmd.Flags().GetBool("follow")
		url := fmt.Sprintf("%s/sandboxes/logs/%s", host, id)
		if follow {
			url += "?follow=true"
		}

		resp, err := http.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to logs: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "Request failed with status %d: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		// Stream to stdout
		if _, err := io.Copy(cmd.OutOrStdout(), resp.Body); err != nil {
			fmt.Fprintf(os.Stderr, "Error streaming logs: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}
