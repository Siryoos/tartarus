package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile  string
	host     string
	token    string
	apiKey   string
	insecure bool
	certFile string
	keyFile  string
	caFile   string
)

type Context struct {
	Host     string `mapstructure:"host"`
	Token    string `mapstructure:"token"`
	Insecure bool   `mapstructure:"insecure"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CaFile   string `mapstructure:"ca_file"`
}

type Config struct {
	CurrentContext string             `mapstructure:"current-context"`
	Contexts       map[string]Context `mapstructure:"contexts"`
}

var rootCmd = &cobra.Command{
	Use:   "tartarus",
	Short: "Tartarus CLI",
	Long:  `A developer-facing tool to interact with the Olympus API.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Load config from active context if not set by flags
		currentContext := viper.GetString("current-context")
		if currentContext == "" {
			currentContext = "default"
		}

		ctxPath := fmt.Sprintf("contexts.%s.", currentContext)

		if host == "" || host == "http://localhost:8080" { // Default value check
			if v := viper.GetString(ctxPath + "host"); v != "" {
				host = v
			}
		}
		if token == "" {
			if v := viper.GetString(ctxPath + "token"); v != "" {
				token = v
			}
		}
		if !insecure {
			if v := viper.GetBool(ctxPath + "insecure"); v {
				insecure = v
			}
		}
		if certFile == "" {
			if v := viper.GetString(ctxPath + "cert_file"); v != "" {
				certFile = v
			}
		}
		if keyFile == "" {
			if v := viper.GetString(ctxPath + "key_file"); v != "" {
				keyFile = v
			}
		}
		if caFile == "" {
			if v := viper.GetString(ctxPath + "ca_file"); v != "" {
				caFile = v
			}
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tartarus/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&host, "host", "http://localhost:8080", "Olympus API URL")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "Bearer token for authentication")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API Key for authentication (alias for token)")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Skip TLS verification")
	rootCmd.PersistentFlags().StringVar(&certFile, "cert", "", "Client certificate file for mTLS")
	rootCmd.PersistentFlags().StringVar(&keyFile, "key", "", "Client key file for mTLS")
	rootCmd.PersistentFlags().StringVar(&caFile, "ca", "", "CA certificate file for mTLS")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		configDir := filepath.Join(home, ".tartarus")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		viper.AddConfigPath(configDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// Config file found and successfully parsed
	}
}
