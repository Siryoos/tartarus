package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "View current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		settings := viper.AllSettings()
		for k, v := range settings {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %v\n", k, v)
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]
		viper.Set(key, value)
		if err := viper.WriteConfig(); err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if errors.As(err, &configFileNotFoundError) || os.IsNotExist(err) {
				if err := viper.SafeWriteConfig(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %s to %s\n", key, value)
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		val := viper.Get(key)
		if val == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "Not set")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), val)
		}
	},
}

func init() {
	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configUseContextCmd)
	configCmd.AddCommand(configGetContextsCmd)
	configCmd.AddCommand(configSetContextCmd)
	rootCmd.AddCommand(configCmd)
}

var configUseContextCmd = &cobra.Command{
	Use:   "use-context [name]",
	Short: "Switch to a different configuration context",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		contextName := args[0]
		viper.Set("current-context", contextName)
		if err := viper.WriteConfig(); err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if errors.As(err, &configFileNotFoundError) || os.IsNotExist(err) {
				if err := viper.SafeWriteConfig(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q\n", contextName)
	},
}

var configGetContextsCmd = &cobra.Command{
	Use:   "get-contexts",
	Short: "List all available configuration contexts",
	Run: func(cmd *cobra.Command, args []string) {
		var config Config
		if err := viper.Unmarshal(&config); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error parsing config: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Current Context: %s\n\n", config.CurrentContext)
		fmt.Fprintln(cmd.OutOrStdout(), "Available Contexts:")
		for name, ctx := range config.Contexts {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s (Host: %s)\n", name, ctx.Host)
		}
	},
}

var configSetContextCmd = &cobra.Command{
	Use:   "set-context [name] [key=value]",
	Short: "Set a value in a configuration context",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		contextName := args[0]
		kv := args[1]

		// Simple key=value parsing
		var key, value string
		for i, char := range kv {
			if char == '=' {
				key = kv[:i]
				value = kv[i+1:]
				break
			}
		}

		if key == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "Invalid format, use key=value")
			os.Exit(1)
		}

		// Map user-friendly keys to config keys
		configKey := fmt.Sprintf("contexts.%s.%s", contextName, key)

		viper.Set(configKey, value)
		if err := viper.WriteConfig(); err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if errors.As(err, &configFileNotFoundError) || os.IsNotExist(err) {
				if err := viper.SafeWriteConfig(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Context %q updated: %s=%s\n", contextName, key, value)
	},
}
