package persephone

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// MetricsCollector defines the interface for collecting historical usage data
type MetricsCollector interface {
	// QueryRange fetches metrics within a time range
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]*UsageRecord, error)
}

// PrometheusCollector implements MetricsCollector for Prometheus
type PrometheusCollector struct {
	api v1.API
}

// NewPrometheusCollector creates a new collector using the given address
func NewPrometheusCollector(address string) (*PrometheusCollector, error) {
	client, err := api.NewClient(api.Config{
		Address: address,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &PrometheusCollector{
		api: v1.NewAPI(client),
	}, nil
}

// QueryRange fetches metrics from Prometheus
func (c *PrometheusCollector) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]*UsageRecord, error) {
	r := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	result, warnings, err := c.api.QueryRange(ctx, query, r)
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}
	if len(warnings) > 0 {
		// Log warnings? For now just ignore or maybe return via error if critical
	}

	matrix, ok := result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unexpected result format: %T", result)
	}

	var records []*UsageRecord

	// We assume the query returns a single series representing the aggregate count
	// If it returns multiple, we might need to sum them or handle them differently.
	// For now, let's assume the query is constructed to return the total count.
	for _, stream := range matrix {
		for _, pair := range stream.Values {
			records = append(records, &UsageRecord{
				Timestamp: pair.Timestamp.Time(),
				ActiveVMs: int(pair.Value),
			})
		}
	}

	return records, nil
}
