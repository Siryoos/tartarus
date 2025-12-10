package thanatos

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// Exporter handles pre-termination data export.
type Exporter struct {
	Runtime   tartarus.SandboxRuntime
	Store     erebus.Store
	ExportDir string
	now       func() time.Time
}

// NewExporter creates a new data exporter.
func NewExporter(runtime tartarus.SandboxRuntime, store erebus.Store, exportDir string) *Exporter {
	if exportDir == "" {
		exportDir = os.TempDir()
	}
	return &Exporter{
		Runtime:   runtime,
		Store:     store,
		ExportDir: exportDir,
		now:       time.Now,
	}
}

// ExportResult captures export outcomes.
type ExportResult struct {
	SandboxID    domain.SandboxID `json:"sandbox_id"`
	LogsKey      string           `json:"logs_key,omitempty"`
	ArtifactsKey string           `json:"artifacts_key,omitempty"`
	ExportedAt   time.Time        `json:"exported_at"`
	LogsSize     int64            `json:"logs_size,omitempty"`
	Error        string           `json:"error,omitempty"`
}

// ExportLogs exports sandbox logs to storage.
func (e *Exporter) ExportLogs(ctx context.Context, id domain.SandboxID) (*ExportResult, error) {
	result := &ExportResult{
		SandboxID:  id,
		ExportedAt: e.now(),
	}

	// Create temp file for logs
	tmpFile, err := os.CreateTemp(e.ExportDir, fmt.Sprintf("logs-%s-*.log", id))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		return result, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Stream logs to temp file
	if err := e.Runtime.StreamLogs(ctx, id, tmpFile, false); err != nil {
		result.Error = fmt.Sprintf("failed to stream logs: %v", err)
		return result, fmt.Errorf("failed to stream logs: %w", err)
	}

	// Get file size
	if stat, err := tmpFile.Stat(); err == nil {
		result.LogsSize = stat.Size()
	}

	// Seek back to beginning
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		result.Error = fmt.Sprintf("failed to seek logs file: %v", err)
		return result, fmt.Errorf("failed to seek logs file: %w", err)
	}

	// Upload to store
	key := fmt.Sprintf("exports/%s/logs/%d.log", id, e.now().UnixNano())
	if err := e.Store.Put(ctx, key, tmpFile); err != nil {
		result.Error = fmt.Sprintf("failed to upload logs: %v", err)
		return result, fmt.Errorf("failed to upload logs: %w", err)
	}

	result.LogsKey = key
	return result, nil
}

// ExportAll exports both logs and artifacts.
func (e *Exporter) ExportAll(ctx context.Context, id domain.SandboxID, exportLogs, exportArtifacts bool) (*ExportResult, error) {
	result := &ExportResult{
		SandboxID:  id,
		ExportedAt: e.now(),
	}

	if exportLogs {
		logsResult, err := e.ExportLogs(ctx, id)
		if err != nil {
			// Don't fail completely, just record the error
			result.Error = logsResult.Error
		} else {
			result.LogsKey = logsResult.LogsKey
			result.LogsSize = logsResult.LogsSize
		}
	}

	// Artifacts export can be added here when artifact storage is implemented
	// For now, logs are the primary export target

	return result, nil
}

// ExportForTermination performs all required exports based on policy.
func (e *Exporter) ExportForTermination(ctx context.Context, id domain.SandboxID, policy *GracePolicy) (*ExportResult, error) {
	if policy == nil {
		return &ExportResult{
			SandboxID:  id,
			ExportedAt: e.now(),
		}, nil
	}

	return e.ExportAll(ctx, id, policy.ExportLogs, policy.ExportArtifacts)
}
