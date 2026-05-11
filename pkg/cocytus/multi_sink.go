package cocytus

import (
	"context"
	"errors"
	"fmt"
)

// MultiSink fans out every Write call to all registered sinks and collects
// errors. If at least one sink succeeds the overall error budget is determined
// by the number of failures: all-failing is an error, partial failure returns
// a joined error that callers can inspect but choose to tolerate.
type MultiSink struct {
	sinks []Sink
}

// NewMultiSink creates a MultiSink wrapping the provided sinks.
// At least one sink must be supplied.
func NewMultiSink(sinks ...Sink) (*MultiSink, error) {
	if len(sinks) == 0 {
		return nil, fmt.Errorf("cocytus: NewMultiSink requires at least one sink")
	}
	return &MultiSink{sinks: sinks}, nil
}

// Write delivers rec to every registered sink. Errors from individual sinks
// are joined together with [errors.Join] so callers can decide how strictly to
// enforce success.
func (m *MultiSink) Write(ctx context.Context, rec *Record) error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Write(ctx, rec); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
