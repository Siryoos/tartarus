package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	host     string
	token    string
	apiKey   string
	insecure bool
	certFile string
	keyFile  string
	caFile   string
)

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
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "Bearer token for authentication")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API Key for authentication (alias for token)")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Skip TLS verification")
	rootCmd.PersistentFlags().StringVar(&certFile, "cert", "", "Client certificate file for mTLS")
	rootCmd.PersistentFlags().StringVar(&keyFile, "key", "", "Client key file for mTLS")
	rootCmd.PersistentFlags().StringVar(&caFile, "ca", "", "CA certificate file for mTLS")
}
