//go:build !linux
// +build !linux

package plugins

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Loader stub for non-Linux platforms.
// Go's plugin system only works on Linux.
type Loader struct {
	logger     hermes.Logger
	pluginsDir string
}

// LoadedPlugin represents a loaded plugin (stub).
type LoadedPlugin struct {
	Manifest *Manifest
	Plugin   Plugin
	Path     string
}

// NewLoader creates a new plugin loader (stub).
func NewLoader(logger hermes.Logger, pluginsDir string) *Loader {
	return &Loader{
		logger:     logger,
		pluginsDir: pluginsDir,
	}
}

// DiscoverAndLoad is a no-op on non-Linux platforms.
func (l *Loader) DiscoverAndLoad(ctx context.Context) error {
	l.logger.Info(ctx, "Plugin loading not supported on this platform", nil)
	return nil
}

// LoadPlugin returns an error on non-Linux platforms.
func (l *Loader) LoadPlugin(ctx context.Context, pluginDir string) error {
	return fmt.Errorf("plugin loading not supported on this platform (Go plugins require Linux)")
}

// UnloadPlugin returns an error on non-Linux platforms.
func (l *Loader) UnloadPlugin(ctx context.Context, name string) error {
	return fmt.Errorf("plugin unloading not supported on this platform")
}

// GetPlugin returns nil on non-Linux platforms.
func (l *Loader) GetPlugin(name string) (*LoadedPlugin, bool) {
	return nil, false
}

// ListPlugins returns empty on non-Linux platforms.
func (l *Loader) ListPlugins() []*LoadedPlugin {
	return nil
}

// GetJudgePlugins returns empty on non-Linux platforms.
func (l *Loader) GetJudgePlugins() []JudgePlugin {
	return nil
}

// GetFuryPlugins returns empty on non-Linux platforms.
func (l *Loader) GetFuryPlugins() []FuryPlugin {
	return nil
}

// Close is a no-op on non-Linux platforms.
func (l *Loader) Close(ctx context.Context) {}
