package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var host string

var rootCmd = &cobra.Command{
	Use:   "tartarus",
	Short: "Tartarus CLI",
	Long:  `A developer-facing tool to interact with the Olympus API.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&host, "host", "http://localhost:8080", "Olympus API URL")
}
