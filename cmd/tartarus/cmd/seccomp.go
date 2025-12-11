package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tartarus-sandbox/tartarus/pkg/typhon"
)

var (
	seccompTemplateID string
	seccompStraceFile string
	seccompOutFile    string
)

var seccompCmd = &cobra.Command{
	Use:   "seccomp",
	Short: "Manage and generate seccomp profiles",
	Long:  `Generate secure seccomp profiles for Tartarus sandboxes based on templates or strace logs.`,
}

var seccompGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a seccomp profile",
	Long: `Generate a seccomp profile either from a predefined template or by analyzing a strace log.
	
Examples:
  tartarus seccomp generate --template python-ds --output python.json
  tartarus seccomp generate --strace app.strace --output app_profile.json`,
	Run: func(cmd *cobra.Command, args []string) {
		if seccompTemplateID == "" && seccompStraceFile == "" {
			fmt.Println("Error: either --template or --strace must be specified")
			os.Exit(1)
		}

		var extraSyscalls []string

		// Analyze strace if provided
		if seccompStraceFile != "" {
			f, err := os.Open(seccompStraceFile)
			if err != nil {
				fmt.Printf("Error opening strace file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()

			syscalls, err := typhon.AnalyzeStrace(f)
			if err != nil {
				fmt.Printf("Error analyzing strace: %v\n", err)
				os.Exit(1)
			}
			extraSyscalls = append(extraSyscalls, syscalls...)
			fmt.Printf("Detected %d syscalls from strace\n", len(syscalls))
		}

		// Use the generator
		gen := typhon.NewSeccompProfileGenerator()

		// If no template is specified, we can use "minimal" or empty.
		// If template IS specified, we use it.
		// The generator's GenerateProfile method takes (templateID, extraSyscalls).
		// If templateID is empty/unknown it just uses base syscalls + extras.

		profile, err := gen.GenerateProfile(seccompTemplateID, extraSyscalls)
		if err != nil {
			fmt.Printf("Error generating profile: %v\n", err)
			os.Exit(1)
		}

		jsonOut, err := profile.ToJSON()
		if err != nil {
			fmt.Printf("Error marshaling profile: %v\n", err)
			os.Exit(1)
		}

		if seccompOutFile != "" {
			if err := os.WriteFile(seccompOutFile, []byte(jsonOut), 0644); err != nil {
				fmt.Printf("Error writing output file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Profile written to %s\n", seccompOutFile)
		} else {
			fmt.Println(jsonOut)
		}
	},
}

var seccompInspectCmd = &cobra.Command{
	Use:   "inspect <file>",
	Short: "Inspect a seccomp profile file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	},
}

func init() {
	rootCmd.AddCommand(seccompCmd)
	seccompCmd.AddCommand(seccompGenerateCmd)
	seccompCmd.AddCommand(seccompInspectCmd)

	seccompGenerateCmd.Flags().StringVar(&seccompTemplateID, "template", "", "Template ID (e.g. python-ds, nodejs)")
	seccompGenerateCmd.Flags().StringVar(&seccompStraceFile, "strace", "", "Path to strace log file to analyze")
	seccompGenerateCmd.Flags().StringVar(&seccompOutFile, "output", "", "Output file path (default stdout)")
}
