package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/plugins"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Tartarus plugins",
	Long:  `Install, list, and remove plugins for custom judges and furies.`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	RunE:  runPluginList,
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <path|url>",
	Short: "Install a plugin from local path or URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginInstall,
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed plugin",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginRemove,
}

var pluginInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show information about a plugin",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginInfo,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
	pluginCmd.AddCommand(pluginInfoCmd)
}

func getPluginsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tartarus", "plugins")
}

func runPluginList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := hermes.NewNoopLogger()
	registry := plugins.NewRegistry(logger, getPluginsDir())

	if err := registry.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize plugin registry: %w", err)
	}
	defer registry.Close(ctx)

	pluginList := registry.ListPlugins()

	if len(pluginList) == 0 {
		fmt.Println("No plugins installed.")
		fmt.Println("\nInstall plugins with:")
		fmt.Println("  tartarus plugin install <path>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tTYPE\tAUTHOR\tDESCRIPTION")
	for _, p := range pluginList {
		desc := p.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Version, p.Type, p.Author, desc)
	}
	w.Flush()

	return nil
}

func runPluginInstall(cmd *cobra.Command, args []string) error {
	source := args[0]
	pluginsDir := getPluginsDir()

	// Check if source is a URL
	if isURL(source) {
		return installFromURL(source, pluginsDir)
	}

	// Check if source is a local path
	return installFromPath(source, pluginsDir)
}

func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}

func installFromPath(sourcePath, pluginsDir string) error {
	// Load manifest to get plugin name
	manifestPath := filepath.Join(sourcePath, "manifest.yaml")
	manifest, err := plugins.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Create target directory
	targetDir := filepath.Join(pluginsDir, manifest.Metadata.Name)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// Copy all files
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		src := filepath.Join(sourcePath, entry.Name())
		dst := filepath.Join(targetDir, entry.Name())

		if entry.IsDir() {
			continue // Skip subdirectories for now
		}

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
		}
	}

	fmt.Printf("✓ Installed plugin '%s' v%s\n", manifest.Metadata.Name, manifest.Metadata.Version)
	return nil
}

func installFromURL(url, pluginsDir string) error {
	// Download to temp directory
	tmpDir, err := os.MkdirTemp("", "tartarus-plugin-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download manifest first
	manifestURL := url + "/manifest.yaml"
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	if err := downloadFile(manifestURL, manifestPath); err != nil {
		return fmt.Errorf("failed to download manifest: %w", err)
	}

	manifest, err := plugins.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Download the plugin binary
	binaryURL := url + "/" + manifest.Spec.EntryPoint
	binaryPath := filepath.Join(tmpDir, manifest.Spec.EntryPoint)

	fmt.Printf("Downloading %s...\n", manifest.Metadata.Name)
	if err := downloadFile(binaryURL, binaryPath); err != nil {
		return fmt.Errorf("failed to download plugin binary: %w", err)
	}

	// Install from temp directory
	return installFromPath(tmpDir, pluginsDir)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func runPluginRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	pluginsDir := getPluginsDir()
	pluginDir := filepath.Join(pluginsDir, name)

	// Check if plugin exists
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin '%s' not found", name)
	}

	// Remove plugin directory
	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin: %w", err)
	}

	fmt.Printf("✓ Removed plugin '%s'\n", name)
	return nil
}

func runPluginInfo(cmd *cobra.Command, args []string) error {
	name := args[0]
	pluginsDir := getPluginsDir()
	manifestPath := filepath.Join(pluginsDir, name, "manifest.yaml")

	manifest, err := plugins.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load plugin manifest: %w", err)
	}

	info := map[string]any{
		"name":        manifest.Metadata.Name,
		"version":     manifest.Metadata.Version,
		"author":      manifest.Metadata.Author,
		"description": manifest.Metadata.Description,
		"type":        manifest.Spec.Type,
		"entryPoint":  manifest.Spec.EntryPoint,
		"config":      manifest.Spec.Config,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}
