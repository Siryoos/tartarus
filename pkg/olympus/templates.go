package olympus

import (
	"context"
	"errors"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

var ErrTemplateNotFound = errors.New("template not found")

// TemplateManager manages the lifecycle and retrieval of sandbox templates.
type TemplateManager interface {
	GetTemplate(ctx context.Context, id domain.TemplateID) (*domain.TemplateSpec, error)
	ListTemplates(ctx context.Context) ([]*domain.TemplateSpec, error)
	RegisterTemplate(ctx context.Context, tpl *domain.TemplateSpec) error
}

// MemoryTemplateManager is an in-memory implementation of TemplateManager.
type MemoryTemplateManager struct {
	mu        sync.RWMutex
	templates map[domain.TemplateID]*domain.TemplateSpec
}

func NewMemoryTemplateManager() *MemoryTemplateManager {
	return &MemoryTemplateManager{
		templates: make(map[domain.TemplateID]*domain.TemplateSpec),
	}
}

func (m *MemoryTemplateManager) GetTemplate(ctx context.Context, id domain.TemplateID) (*domain.TemplateSpec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if tpl, ok := m.templates[id]; ok {
		return tpl, nil
	}
	return nil, ErrTemplateNotFound
}

func (m *MemoryTemplateManager) ListTemplates(ctx context.Context) ([]*domain.TemplateSpec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*domain.TemplateSpec, 0, len(m.templates))
	for _, tpl := range m.templates {
		list = append(list, tpl)
	}
	return list, nil
}

func (m *MemoryTemplateManager) RegisterTemplate(ctx context.Context, tpl *domain.TemplateSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.templates[tpl.ID] = tpl
	return nil
}
