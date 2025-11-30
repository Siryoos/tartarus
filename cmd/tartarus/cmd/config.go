package cmd

import (
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
			fmt.Printf("%s: %v\n", k, v)
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
			// If config file doesn't exist, create it
			if os.IsNotExist(err) {
				// Try to write to default location
				// This is tricky without knowing where viper looked.
				// But usually safe to just print error for now or use SafeWriteConfig
				if err := viper.SafeWriteConfig(); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("Set %s to %s\n", key, value)
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
			fmt.Println("Not set")
		} else {
			fmt.Println(val)
		}
	},
}

func init() {
	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	rootCmd.AddCommand(configCmd)
}
