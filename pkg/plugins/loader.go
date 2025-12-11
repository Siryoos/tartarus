//go:build linux
// +build linux

package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Loader discovers and loads plugins from the filesystem.
type Loader struct {
	logger     hermes.Logger
	pluginsDir string

	mu     sync.RWMutex
	loaded map[string]*LoadedPlugin
}

// LoadedPlugin represents a successfully loaded plugin.
type LoadedPlugin struct {
	Manifest *Manifest
	Plugin   Plugin
	Path     string
}

// NewLoader creates a new plugin loader.
func NewLoader(logger hermes.Logger, pluginsDir string) *Loader {
	if pluginsDir == "" {
		home, _ := os.UserHomeDir()
		pluginsDir = filepath.Join(home, ".tartarus", "plugins")
	}

	return &Loader{
		logger:     logger,
		pluginsDir: pluginsDir,
		loaded:     make(map[string]*LoadedPlugin),
	}
}

// DiscoverAndLoad scans the plugins directory and loads all valid plugins.
func (l *Loader) DiscoverAndLoad(ctx context.Context) error {
	// Create plugins directory if it doesn't exist
	if err := os.MkdirAll(l.pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	entries, err := os.ReadDir(l.pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(l.pluginsDir, entry.Name())
		if err := l.LoadPlugin(ctx, pluginPath); err != nil {
			l.logger.Error(ctx, "Failed to load plugin", map[string]any{
				"path":  pluginPath,
				"error": err.Error(),
			})
			// Continue loading other plugins
		}
	}

	return nil
}

// LoadPlugin loads a single plugin from the given directory.
func (l *Loader) LoadPlugin(ctx context.Context, pluginDir string) error {
	// Load manifest
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Check if already loaded
	l.mu.RLock()
	if _, exists := l.loaded[manifest.Metadata.Name]; exists {
		l.mu.RUnlock()
		return fmt.Errorf("plugin '%s' already loaded", manifest.Metadata.Name)
	}
	l.mu.RUnlock()

	// Load the shared object
	soPath := filepath.Join(pluginDir, manifest.Spec.EntryPoint)
	p, err := plugin.Open(soPath)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	// Look up the plugin symbol
	sym, err := p.Lookup(PluginSymbol)
	if err != nil {
		return fmt.Errorf("plugin missing %s symbol: %w", PluginSymbol, err)
	}

	// Assert to Plugin interface
	plug, ok := sym.(Plugin)
	if !ok {
		// Try pointer to Plugin
		plugPtr, ok := sym.(*Plugin)
		if !ok {
			return fmt.Errorf("symbol %s does not implement Plugin interface", PluginSymbol)
		}
		plug = *plugPtr
	}

	// Validate type matches manifest
	if plug.Type() != manifest.Spec.Type {
		return fmt.Errorf("plugin type mismatch: manifest says '%s', plugin says '%s'",
			manifest.Spec.Type, plug.Type())
	}

	// Initialize plugin
	if err := plug.Init(manifest.Spec.Config); err != nil {
		return fmt.Errorf("plugin init failed: %w", err)
	}

	// Register
	l.mu.Lock()
	l.loaded[manifest.Metadata.Name] = &LoadedPlugin{
		Manifest: manifest,
		Plugin:   plug,
		Path:     pluginDir,
	}
	l.mu.Unlock()

	l.logger.Info(ctx, "Loaded plugin", map[string]any{
		"name":    manifest.Metadata.Name,
		"version": manifest.Metadata.Version,
		"type":    manifest.Spec.Type,
	})

	return nil
}

// UnloadPlugin unloads a plugin by name.
func (l *Loader) UnloadPlugin(ctx context.Context, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	loaded, exists := l.loaded[name]
	if !exists {
		return fmt.Errorf("plugin '%s' not found", name)
	}

	if err := loaded.Plugin.Close(); err != nil {
		l.logger.Error(ctx, "Plugin close error", map[string]any{
			"name":  name,
			"error": err.Error(),
		})
	}

	delete(l.loaded, name)

	l.logger.Info(ctx, "Unloaded plugin", map[string]any{
		"name": name,
	})

	return nil
}

// GetPlugin returns a loaded plugin by name.
func (l *Loader) GetPlugin(name string) (*LoadedPlugin, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p, ok := l.loaded[name]
	return p, ok
}

// ListPlugins returns all loaded plugins.
func (l *Loader) ListPlugins() []*LoadedPlugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*LoadedPlugin, 0, len(l.loaded))
	for _, p := range l.loaded {
		result = append(result, p)
	}
	return result
}

// GetJudgePlugins returns all loaded judge plugins.
func (l *Loader) GetJudgePlugins() []JudgePlugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []JudgePlugin
	for _, p := range l.loaded {
		if jp, ok := p.Plugin.(JudgePlugin); ok {
			result = append(result, jp)
		}
	}
	return result
}

// GetFuryPlugins returns all loaded fury plugins.
func (l *Loader) GetFuryPlugins() []FuryPlugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []FuryPlugin
	for _, p := range l.loaded {
		if fp, ok := p.Plugin.(FuryPlugin); ok {
			result = append(result, fp)
		}
	}
	return result
}

// Close unloads all plugins.
func (l *Loader) Close(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for name, loaded := range l.loaded {
		if err := loaded.Plugin.Close(); err != nil {
			l.logger.Error(ctx, "Plugin close error", map[string]any{
				"name":  name,
				"error": err.Error(),
			})
		}
	}

	l.loaded = make(map[string]*LoadedPlugin)
}
