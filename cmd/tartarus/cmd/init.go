package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize resources",
}

var initTemplateCmd = &cobra.Command{
	Use:   "template [name]",
	Short: "Scaffold a new Tartarus template",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nameStr := args[0]
		baseImage, _ := cmd.Flags().GetString("image")
		dockerfile, _ := cmd.Flags().GetString("dockerfile")

		// Default values
		template := map[string]interface{}{
			"id":           nameStr,
			"name":         nameStr,
			"description":  fmt.Sprintf("Template for %s", nameStr),
			"base_image":   "alpine:latest",
			"kernel_image": "vmlinux-5.10",
			"resources": map[string]interface{}{
				"cpu_milli": 1000,
				"mem_mb":    512,
				"ttl":       "1h",
			},
			"default_env": map[string]string{
				"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		}

		if baseImage != "" {
			fmt.Printf("Inspecting image %s...\n", baseImage)
			template["base_image"] = baseImage
			if err := inspectImage(baseImage, template); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to inspect image: %v\n", err)
			}
		} else if dockerfile != "" {
			fmt.Printf("Parsing Dockerfile %s...\n", dockerfile)
			if err := parseDockerfile(dockerfile, template); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse Dockerfile: %v\n", err)
			}
		}

		data, err := yaml.Marshal(template)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling template: %v\n", err)
			os.Exit(1)
		}

		filename := fmt.Sprintf("%s.yaml", nameStr)
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Template scaffolded to %s\n", filename)
	},
}

func inspectImage(ref string, template map[string]interface{}) error {
	nameRef, err := name.ParseReference(ref)
	if err != nil {
		return err
	}

	img, err := remote.Image(nameRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return err
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return err
	}

	// Extract Env
	envMap := make(map[string]string)
	// Start with existing defaults
	if existing, ok := template["default_env"].(map[string]string); ok {
		for k, v := range existing {
			envMap[k] = v
		}
	}

	for _, e := range configFile.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	template["default_env"] = envMap

	// Extract WorkingDir
	if configFile.Config.WorkingDir != "" {
		// We don't have a direct field for WorkingDir in TemplateSpec yet,
		// but we can add it to metadata or env if needed.
		// For now, let's just log it or maybe add it to env PWD?
		// Or assume the runtime handles it if we add it to the spec later.
		// Let's add it to metadata for now if we had it, but map[string]interface{} is flexible.
		// Let's add it as a custom field, maybe the user will use it.
		template["working_dir"] = configFile.Config.WorkingDir
	}

	// Extract Entrypoint/Cmd
	// This is tricky as Tartarus templates usually have a specific init,
	// but maybe we can use it for the default command?
	// The TemplateSpec doesn't seem to have a default command field in the struct we saw earlier
	// (it had WarmupCommand).
	// But let's check domain.TemplateSpec again.
	// We'll skip for now.

	return nil
}

func parseDockerfile(path string, template map[string]interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	envMap := make(map[string]string)
	if existing, ok := template["default_env"].(map[string]string); ok {
		for k, v := range existing {
			envMap[k] = v
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "FROM ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				template["base_image"] = parts[1]
			}
		} else if strings.HasPrefix(line, "ENV ") {
			// ENV key=value or ENV key value
			parts := strings.Fields(line[4:])
			if len(parts) > 0 {
				// Handle key=value
				if strings.Contains(parts[0], "=") {
					for _, p := range parts {
						kv := strings.SplitN(p, "=", 2)
						if len(kv) == 2 {
							envMap[kv[0]] = kv[1]
						}
					}
				} else if len(parts) >= 2 {
					// ENV key value
					envMap[parts[0]] = parts[1]
				}
			}
		} else if strings.HasPrefix(line, "WORKDIR ") {
			template["working_dir"] = strings.TrimSpace(line[8:])
		}
	}

	template["default_env"] = envMap
	return scanner.Err()
}

func init() {
	initTemplateCmd.Flags().String("image", "", "Base image OCI reference (e.g. alpine:latest)")
	initTemplateCmd.Flags().String("dockerfile", "", "Path to Dockerfile to parse")
	initCmd.AddCommand(initTemplateCmd)
	rootCmd.AddCommand(initCmd)
}
