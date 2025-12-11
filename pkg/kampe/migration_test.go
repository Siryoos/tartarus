package kampe

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// MockLegacyRuntime mocks the LegacyRuntime interface
type MockLegacyRuntime struct {
	mock.Mock
}

func (m *MockLegacyRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	args := m.Called(ctx, req, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockLegacyRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockLegacyRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.SandboxRun), args.Error(1)
}
func (m *MockLegacyRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockLegacyRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockLegacyRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockLegacyRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	args := m.Called(ctx, id, memPath, diskPath)
	return args.Error(0)
}
func (m *MockLegacyRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockLegacyRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(tartarus.VMConfig), args.Get(1).(*domain.SandboxRequest), args.Error(2)
}
func (m *MockLegacyRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	args := m.Called(ctx, id, w, follow)
	return args.Error(0)
}
func (m *MockLegacyRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	args := m.Called(ctx)
	return args.Get(0).(domain.ResourceCapacity), args.Error(1)
}
func (m *MockLegacyRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockLegacyRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error { // Simplified signature match
	return nil
}
func (m *MockLegacyRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return nil
}

// Additional Legacy methods
func (m *MockLegacyRuntime) CanMigrate(ctx context.Context, containerID string) (bool, error) {
	args := m.Called(ctx, containerID)
	return args.Bool(0), args.Error(1)
}
func (m *MockLegacyRuntime) MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(*MigrationPlan), args.Error(1)
}
func (m *MockLegacyRuntime) ExportState(ctx context.Context, containerID string) (*ContainerState, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(*ContainerState), args.Error(1)
}

// MockTargetRuntime mocks the target SandboxRuntime
type MockTargetRuntime struct {
	mock.Mock
}

// Implement only needed methods for target
func (m *MockTargetRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	args := m.Called(ctx, req, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockTargetRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockTargetRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Stubs for interface compliance
func (m *MockTargetRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) { return nil, nil }
func (m *MockTargetRuntime) Pause(ctx context.Context, id domain.SandboxID) error  { return nil }
func (m *MockTargetRuntime) Resume(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockTargetRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, mp, dp string) error {
	return nil
}
func (m *MockTargetRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockTargetRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	return tartarus.VMConfig{}, nil, nil
}
func (m *MockTargetRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	return nil
}
func (m *MockTargetRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	return domain.ResourceCapacity{}, nil
}
func (m *MockTargetRuntime) Wait(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockTargetRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	return nil
}
func (m *MockTargetRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return nil
}

func TestMigrationManager_Migrate_Success(t *testing.T) {
	source := new(MockLegacyRuntime)
	target := new(MockTargetRuntime)
	manager := NewMigrationManager(source, target)
	ctx := context.Background()
	containerID := "test-container"

	// 1. Assess
	plan := &MigrationPlan{
		ContainerID: containerID,
		RiskLevel:   RiskLevelLow,
	}
	source.On("MigrateToMicroVM", ctx, containerID).Return(plan, nil)

	// 2. Pause
	source.On("Pause", ctx, domain.SandboxID(containerID)).Return(nil)

	// 3. Export
	state := &ContainerState{
		ID:    containerID,
		Image: "alpine:latest",
		Config: ContainerConfig{
			Cmd: []string{"sh"},
		},
		Environment: map[string]string{"FOO": "BAR"},
	}
	source.On("ExportState", ctx, containerID).Return(state, nil)

	// 5. Launch
	newRun := &domain.SandboxRun{
		ID:     "new-vm-id",
		Status: domain.RunStatusRunning,
	}
	target.On("Launch", ctx, mock.Anything, mock.Anything).Return(newRun, nil)

	// 6. Verify
	target.On("Inspect", ctx, domain.SandboxID("new-vm-id")).Return(newRun, nil)

	// 7. Cutover
	source.On("Kill", mock.Anything, domain.SandboxID(containerID)).Return(nil)

	// Execute
	result := manager.Migrate(ctx, containerID)

	// Assert
	if result.Error != nil {
		t.Fatalf("Migration failed: %v", result.Error)
	}
	if result.NewID != "new-vm-id" {
		t.Errorf("Expected NewID=new-vm-id, got %s", result.NewID)
	}
	if result.RollbackStatus != "none" {
		t.Errorf("Expected RollbackStatus=none, got %s", result.RollbackStatus)
	}
}

func TestMigrationManager_Migrate_RollbackOnFailure(t *testing.T) {
	source := new(MockLegacyRuntime)
	target := new(MockTargetRuntime)
	manager := NewMigrationManager(source, target)
	ctx := context.Background()
	containerID := "test-container-fail"

	// 1. Assess
	plan := &MigrationPlan{RiskLevel: RiskLevelLow}
	source.On("MigrateToMicroVM", ctx, containerID).Return(plan, nil)

	// 2. Pause
	source.On("Pause", ctx, domain.SandboxID(containerID)).Return(nil)

	// 3. Export
	state := &ContainerState{ID: containerID}
	source.On("ExportState", ctx, containerID).Return(state, nil)

	// 5. Launch FAILS
	target.On("Launch", ctx, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("launch error"))

	// Expect Rollback (Resume)
	source.On("Resume", mock.Anything, domain.SandboxID(containerID)).Return(nil)

	// Execute
	result := manager.Migrate(ctx, containerID)

	// Assert
	if result.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if result.RollbackStatus != "success" {
		t.Errorf("Expected RollbackStatus=success, got %s", result.RollbackStatus)
	}
}
