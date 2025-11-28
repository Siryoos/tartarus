package cocytus

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Record captures failed runs and their lamentations.

type Record struct {
	RunID     domain.SandboxID `json:"run_id"`
	RequestID domain.SandboxID `json:"request_id"`
	Reason    string           `json:"reason"`
	Payload   []byte           `json:"payload"`
	CreatedAt time.Time        `json:"created_at"`
}

// Sink is the interface for Cocytus.

type Sink interface {
	Write(ctx context.Context, rec *Record) error
}
