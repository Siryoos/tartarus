package persephone

import (
	"context"
	"fmt"
	"time"
)

// Ingestor periodically fetches metrics and persists them to history
type Ingestor struct {
	collector MetricsCollector
	store     HistoryStore
	interval  time.Duration
	query     string
}

// IngestorConfig holds configuration for the Ingestor
type IngestorConfig struct {
	Collector MetricsCollector
	Store     HistoryStore
	Interval  time.Duration
	Query     string // Prometheus query string
}

// NewIngestor creates a new Ingestor
func NewIngestor(config IngestorConfig) (*Ingestor, error) {
	if config.Collector == nil {
		return nil, fmt.Errorf("collector is required")
	}
	if config.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if config.Interval <= 0 {
		config.Interval = 5 * time.Minute
	}
	if config.Query == "" {
		// Default query if none provided
		config.Query = "sum(tartarus_active_sandboxes)"
	}

	return &Ingestor{
		collector: config.Collector,
		store:     config.Store,
		interval:  config.Interval,
		query:     config.Query,
	}, nil
}

// Start begins the ingestion loop
func (i *Ingestor) Start(ctx context.Context) error {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()

	// Initial run
	if err := i.ingest(ctx); err != nil {
		// Log error but continue
		// fmt.Printf("Initial ingestion failed: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := i.ingest(ctx); err != nil {
				// Log error (should use a logger in real impl)
				// fmt.Printf("Ingestion failed: %v\n", err)
			}
		}
	}
}

func (i *Ingestor) ingest(ctx context.Context) error {
	// Look back one interval to capture the latest data
	// We might want to look back slightly more to ensure we don't miss anything due to scrape latency
	end := time.Now()
	start := end.Add(-i.interval)

	// Fetch metrics
	records, err := i.collector.QueryRange(ctx, i.query, start, end, time.Minute)
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	// Persist metrics
	if err := i.store.Save(ctx, records); err != nil {
		return fmt.Errorf("failed to save metrics: %w", err)
	}

	return nil
}
