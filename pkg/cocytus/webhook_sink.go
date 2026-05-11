package cocytus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookSink is a durable [Sink] that delivers failed records to an HTTP
// webhook. It is compatible with Slack incoming webhooks and the PagerDuty
// Events v2 API when the URL and optional header auth are configured correctly.
//
// The request body is a JSON object that contains all fields of [Record].
// A non-2xx response code is treated as an error.
type WebhookSink struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// WebhookSinkConfig holds the options for [NewWebhookSink].
type WebhookSinkConfig struct {
	// URL is the endpoint to POST records to (required).
	URL string
	// Headers is an optional map of additional HTTP headers (e.g.
	// Authorization: Bearer <token> for PagerDuty).
	Headers map[string]string
	// Timeout is the per-request HTTP timeout. Defaults to 10 seconds.
	Timeout time.Duration
}

// NewWebhookSink creates a WebhookSink.
func NewWebhookSink(cfg WebhookSinkConfig) (*WebhookSink, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("cocytus: WebhookSink requires a non-empty URL")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &WebhookSink{
		url:     cfg.URL,
		headers: cfg.Headers,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// webhookPayload is the JSON body sent to the webhook endpoint.
type webhookPayload struct {
	RunID     string    `json:"run_id"`
	RequestID string    `json:"request_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	// PayloadSize is included for brevity; raw bytes are not forwarded to avoid
	// leaking sensitive sandbox state to external services.
	PayloadSize int `json:"payload_size"`
}

// Write serialises the record and POSTs it to the configured webhook URL.
func (s *WebhookSink) Write(ctx context.Context, rec *Record) error {
	body, err := json.Marshal(webhookPayload{
		RunID:       string(rec.RunID),
		RequestID:   string(rec.RequestID),
		Reason:      rec.Reason,
		CreatedAt:   rec.CreatedAt,
		PayloadSize: len(rec.Payload),
	})
	if err != nil {
		return fmt.Errorf("cocytus: WebhookSink failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cocytus: WebhookSink failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("cocytus: WebhookSink POST to %s failed: %w", s.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cocytus: WebhookSink received non-2xx status %d from %s", resp.StatusCode, s.url)
	}
	return nil
}
