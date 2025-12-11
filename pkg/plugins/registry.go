package plugins

import (
	"context"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
)

// Registry manages plugin lifecycle and integration with Tartarus components.
type Registry struct {
	loader *Loader
	logger hermes.Logger

	mu            sync.RWMutex
	judgeChain    *judges.Chain
	compositeFury *CompositeFury
}

// NewRegistry creates a new plugin registry.
func NewRegistry(logger hermes.Logger, pluginsDir string) *Registry {
	return &Registry{
		loader:        NewLoader(logger, pluginsDir),
		logger:        logger,
		judgeChain:    &judges.Chain{},
		compositeFury: NewCompositeFury(),
	}
}

// Initialize discovers and loads all plugins, wiring them into the registry.
func (r *Registry) Initialize(ctx context.Context) error {
	if err := r.loader.DiscoverAndLoad(ctx); err != nil {
		return err
	}

	r.rebuildIntegrations()
	return nil
}

// rebuildIntegrations updates the judge chain and fury composite from loaded plugins.
func (r *Registry) rebuildIntegrations() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get all judge plugins and wrap them
	judgePlugins := r.loader.GetJudgePlugins()
	pre, post := WrapJudgePlugins(judgePlugins)
	r.judgeChain = &judges.Chain{Pre: pre, Post: post}

	// Get all fury plugins and wrap them
	furyPlugins := r.loader.GetFuryPlugins()
	wrappedFuries := WrapFuryPlugins(furyPlugins)
	r.compositeFury = NewCompositeFury(wrappedFuries...)
}

// LoadPlugin loads a plugin and updates integrations.
func (r *Registry) LoadPlugin(ctx context.Context, pluginDir string) error {
	if err := r.loader.LoadPlugin(ctx, pluginDir); err != nil {
		return err
	}
	r.rebuildIntegrations()
	return nil
}

// UnloadPlugin unloads a plugin and updates integrations.
func (r *Registry) UnloadPlugin(ctx context.Context, name string) error {
	if err := r.loader.UnloadPlugin(ctx, name); err != nil {
		return err
	}
	r.rebuildIntegrations()
	return nil
}

// GetJudgeChain returns the current judge chain including plugin judges.
func (r *Registry) GetJudgeChain() *judges.Chain {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.judgeChain
}

// GetCompositeFury returns the composite fury including plugin furies.
func (r *Registry) GetCompositeFury() *CompositeFury {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.compositeFury
}

// GetFury returns the composite fury as an erinyes.Fury interface.
func (r *Registry) GetFury() erinyes.Fury {
	return r.GetCompositeFury()
}

// ListPlugins returns info about all loaded plugins.
func (r *Registry) ListPlugins() []PluginInfo {
	loaded := r.loader.ListPlugins()
	info := make([]PluginInfo, 0, len(loaded))
	for _, p := range loaded {
		info = append(info, PluginInfo{
			Name:        p.Manifest.Metadata.Name,
			Version:     p.Manifest.Metadata.Version,
			Type:        p.Manifest.Spec.Type,
			Author:      p.Manifest.Metadata.Author,
			Description: p.Manifest.Metadata.Description,
			Path:        p.Path,
		})
	}
	return info
}

// PluginInfo contains summary information about a loaded plugin.
type PluginInfo struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Type        PluginType `json:"type"`
	Author      string     `json:"author"`
	Description string     `json:"description"`
	Path        string     `json:"path"`
}

// Close shuts down all plugins.
func (r *Registry) Close(ctx context.Context) {
	r.loader.Close(ctx)
}
