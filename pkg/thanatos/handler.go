package thanatos

import (
	"context"
	"errors"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// Phase represents the termination lifecycle stage.
type Phase string

const (
	PhaseInitiated     Phase = "initiated"
	PhaseGraceful      Phase = "graceful_shutdown"
	PhaseCheckpointed  Phase = "checkpointed"
	PhaseKilled        Phase = "killed"
	PhaseCompleted     Phase = "completed"
	PhaseFailed        Phase = "failed"
	defaultGracePeriod       = 5 * time.Second
)

// Options customize termination handling.
type Options struct {
	GracePeriod      time.Duration
	Reason           string
	CreateCheckpoint bool
}

// Result captures the outcome of a termination attempt.
type Result struct {
	SandboxID    domain.SandboxID
	Phase        Phase
	Reason       string
	ExitCode     *int
	Checkpoint   string
	ErrorMessage string
	CompletedAt  time.Time
}

// Handler performs graceful termination with optional checkpointing.
type Handler struct {
	Runtime tartarus.SandboxRuntime
	Hypnos  *hypnos.Manager
	now     func() time.Time
}

// NewHandler constructs a Thanatos handler.
func NewHandler(runtime tartarus.SandboxRuntime, sleeper *hypnos.Manager) *Handler {
	return &Handler{
		Runtime: runtime,
		Hypnos:  sleeper,
		now:     time.Now,
	}
}

// Terminate attempts a graceful shutdown, optionally checkpointing first.
func (h *Handler) Terminate(ctx context.Context, id domain.SandboxID, opts Options) (*Result, error) {
	res := &Result{
		SandboxID: id,
		Phase:     PhaseInitiated,
		Reason:    opts.Reason,
	}

	grace := opts.GracePeriod
	if grace == 0 {
		grace = defaultGracePeriod
	}

	if opts.CreateCheckpoint && h.Hypnos != nil {
		rec, err := h.Hypnos.Sleep(ctx, id, &hypnos.SleepOptions{GracefulShutdown: true})
		if err != nil {
			res.Phase = PhaseFailed
			res.ErrorMessage = err.Error()
			return res, err
		}
		res.Phase = PhaseCheckpointed
		res.Checkpoint = rec.SnapshotKey
		res.CompletedAt = h.now()
		return res, nil
	}

	if err := h.Runtime.Shutdown(ctx, id); err != nil {
		res.Phase = PhaseFailed
		res.ErrorMessage = err.Error()
		return res, err
	}
	res.Phase = PhaseGraceful

	waitCtx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()

	if err := h.Runtime.Wait(waitCtx, id); err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			_ = h.Runtime.Kill(context.Background(), id)
			res.Phase = PhaseKilled
			res.ErrorMessage = "grace period exceeded; sandbox killed"
			return res, errors.New(res.ErrorMessage)
		}
		res.Phase = PhaseFailed
		res.ErrorMessage = err.Error()
		return res, err
	}

	if run, err := h.Runtime.Inspect(ctx, id); err == nil {
		res.ExitCode = run.ExitCode
	}
	res.Phase = PhaseCompleted
	res.CompletedAt = h.now()

	return res, nil
}
