package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage sandbox templates",
	Long:  `Search, install, and publish sandbox templates from the marketplace.`,
}

var templateSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for templates in the marketplace",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTemplateSearch,
}

var templateInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a template from the marketplace",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateInstall,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed templates",
	RunE:  runTemplateList,
}

var templateInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show information about a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateInfo,
}

var templateValidateCmd = &cobra.Command{
	Use:   "validate <path>",
	Short: "Validate a template manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateValidate,
}

var (
	templateCategory string
	templateTags     string
)

func init() {
	rootCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(templateSearchCmd)
	templateCmd.AddCommand(templateInstallCmd)
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateInfoCmd)
	templateCmd.AddCommand(templateValidateCmd)

	templateSearchCmd.Flags().StringVar(&templateCategory, "category", "", "Filter by category")
	templateSearchCmd.Flags().StringVar(&templateTags, "tags", "", "Filter by tags (comma-separated)")
}

// TemplateRegistry represents the marketplace registry
type TemplateRegistry struct {
	Templates  []TemplateEntry `yaml:"templates"`
	Categories []CategoryEntry `yaml:"categories"`
}

type TemplateEntry struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Category    string   `yaml:"category"`
	Tags        []string `yaml:"tags"`
	Downloads   int      `yaml:"downloads"`
	Rating      float64  `yaml:"rating"`
	Verified    bool     `yaml:"verified"`
	URL         string   `yaml:"url"`
}

type CategoryEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
}

func loadRegistry() (*TemplateRegistry, error) {
	// Look for registry in known locations
	paths := []string{
		"ecosystem/marketplace/registry.yaml",
		"registry.yaml",
	}

	// Add path relative to executable
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	paths = append(paths, filepath.Join(exeDir, "..", "ecosystem", "marketplace", "registry.yaml"))

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			var reg TemplateRegistry
			if err := yaml.Unmarshal(data, &reg); err != nil {
				return nil, fmt.Errorf("failed to parse registry: %w", err)
			}
			return &reg, nil
		}
	}

	return nil, fmt.Errorf("registry not found")
}

func getTemplatesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tartarus", "templates")
}

func runTemplateSearch(cmd *cobra.Command, args []string) error {
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	query := ""
	if len(args) > 0 {
		query = strings.ToLower(args[0])
	}

	var matches []TemplateEntry
	for _, t := range registry.Templates {
		// Filter by query
		if query != "" {
			nameMatch := strings.Contains(strings.ToLower(t.Name), query)
			descMatch := strings.Contains(strings.ToLower(t.Description), query)
			tagMatch := false
			for _, tag := range t.Tags {
				if strings.Contains(strings.ToLower(tag), query) {
					tagMatch = true
					break
				}
			}
			if !nameMatch && !descMatch && !tagMatch {
				continue
			}
		}

		// Filter by category
		if templateCategory != "" && t.Category != templateCategory {
			continue
		}

		// Filter by tags
		if templateTags != "" {
			filterTags := strings.Split(templateTags, ",")
			hasTag := false
			for _, ft := range filterTags {
				for _, tt := range t.Tags {
					if strings.EqualFold(strings.TrimSpace(ft), tt) {
						hasTag = true
						break
					}
				}
			}
			if !hasTag {
				continue
			}
		}

		matches = append(matches, t)
	}

	// Sort by downloads
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Downloads > matches[j].Downloads
	})

	if len(matches) == 0 {
		fmt.Println("No templates found matching your criteria.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tCATEGORY\tRATING\tDOWNLOADS\tVERIFIED")
	for _, t := range matches {
		verified := ""
		if t.Verified {
			verified = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.1f\t%d\t%s\n",
			t.Name, t.Version, t.Category, t.Rating, t.Downloads, verified)
	}
	w.Flush()

	return nil
}

func runTemplateInstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	templatesDir := getTemplatesDir()

	// Load registry to find template
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	var template *TemplateEntry
	for _, t := range registry.Templates {
		if t.Name == name {
			template = &t
			break
		}
	}

	if template == nil {
		return fmt.Errorf("template '%s' not found in marketplace", name)
	}

	// Create templates directory
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return fmt.Errorf("failed to create templates directory: %w", err)
	}

	// Look for local template file first
	localPaths := []string{
		fmt.Sprintf("ecosystem/marketplace/templates/%s.yaml", name),
		fmt.Sprintf("templates/%s.yaml", name),
	}

	for _, p := range localPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			destPath := filepath.Join(templatesDir, name+".yaml")
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return fmt.Errorf("failed to install template: %w", err)
			}
			fmt.Printf("✓ Installed template '%s' v%s\n", name, template.Version)
			fmt.Printf("  Use with: tartarus run --template %s\n", name)
			return nil
		}
	}

	return fmt.Errorf("template '%s' source not found locally; remote download not implemented", name)
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	templatesDir := getTemplatesDir()

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No templates installed.")
			fmt.Println("\nInstall templates with:")
			fmt.Println("  tartarus template install <name>")
			return nil
		}
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No templates installed.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tPATH")
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		path := filepath.Join(templatesDir, entry.Name())

		// Try to read version from file
		version := "unknown"
		data, err := os.ReadFile(path)
		if err == nil {
			var t struct {
				Metadata struct {
					Version string `yaml:"version"`
				} `yaml:"metadata"`
			}
			if yaml.Unmarshal(data, &t) == nil && t.Metadata.Version != "" {
				version = t.Metadata.Version
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", name, version, path)
	}
	w.Flush()

	return nil
}

func runTemplateInfo(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Try installed templates first
	templatesDir := getTemplatesDir()
	installedPath := filepath.Join(templatesDir, name+".yaml")

	var data []byte
	var err error

	if data, err = os.ReadFile(installedPath); err != nil {
		// Try marketplace
		localPaths := []string{
			fmt.Sprintf("ecosystem/marketplace/templates/%s.yaml", name),
		}
		for _, p := range localPaths {
			if data, err = os.ReadFile(p); err == nil {
				break
			}
		}
	}

	if len(data) == 0 {
		return fmt.Errorf("template '%s' not found", name)
	}

	// Pretty print as JSON
	var template map[string]any
	if err := yaml.Unmarshal(data, &template); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(template)
}

func runTemplateValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML
	var template struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name        string `yaml:"name"`
			Version     string `yaml:"version"`
			Author      string `yaml:"author"`
			Description string `yaml:"description"`
		} `yaml:"metadata"`
		Spec struct {
			BaseImage string   `yaml:"baseImage"`
			Packages  []string `yaml:"packages"`
			Resources struct {
				CPU    int `yaml:"cpu"`
				Memory int `yaml:"memory"`
			} `yaml:"resources"`
		} `yaml:"spec"`
	}

	if err := yaml.Unmarshal(data, &template); err != nil {
		fmt.Printf("✗ Invalid YAML: %v\n", err)
		return err
	}

	var errors []string

	// Validate required fields
	if template.APIVersion == "" {
		errors = append(errors, "missing apiVersion")
	}
	if template.Kind != "Template" {
		errors = append(errors, "kind must be 'Template'")
	}
	if template.Metadata.Name == "" {
		errors = append(errors, "missing metadata.name")
	}
	if template.Metadata.Version == "" {
		errors = append(errors, "missing metadata.version")
	}
	if template.Spec.BaseImage == "" {
		errors = append(errors, "missing spec.baseImage")
	}

	if len(errors) > 0 {
		fmt.Println("✗ Validation failed:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("validation failed with %d errors", len(errors))
	}

	fmt.Printf("✓ Template '%s' v%s is valid\n", template.Metadata.Name, template.Metadata.Version)
	return nil
}
