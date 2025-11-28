package charon

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// FerryMiddleware wraps an HTTP handler with Charon ferry logic.
type FerryMiddleware struct {
	ferry Ferry
}

// NewFerryMiddleware creates a new ferry middleware.
func NewFerryMiddleware(ferry Ferry) *FerryMiddleware {
	return &FerryMiddleware{
		ferry: ferry,
	}
}

// Handler returns an HTTP handler that ferries requests through Charon.
func (m *FerryMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add request metadata to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, "remote_ip", r.RemoteAddr)

		// Extract tenant ID from header if present
		if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
			ctx = context.WithValue(ctx, "tenant_id", tenantID)
		}

		// Extract identity from header if present
		if identityID := r.Header.Get("X-Identity-ID"); identityID != "" {
			ctx = context.WithValue(ctx, "identity_id", identityID)
		}

		// Ferry the request
		start := time.Now()
		resp, err := m.ferry.Cross(ctx, r.WithContext(ctx))
		duration := time.Since(start)

		if err != nil {
			// Handle ferry errors
			httpErr := ToHTTPError(err)
			http.Error(w, httpErr.Message, httpErr.HTTPStatusCode())
			return
		}

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Add ferry metadata headers
		w.Header().Set("X-Ferry-Duration", duration.String())

		// Write status code
		w.WriteHeader(resp.StatusCode)

		// Copy response body
		if resp.Body != nil {
			defer resp.Body.Close()
			// Use io.Copy to stream the response
			// This is handled by the response recorder in the ferry
		}
	})
}

// HealthHandler returns an HTTP handler for health checks.
func (m *FerryMiddleware) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health, err := m.ferry.Health(r.Context())
		if err != nil {
			http.Error(w, "Health check failed", http.StatusInternalServerError)
			return
		}

		// Determine HTTP status based on health
		statusCode := http.StatusOK
		switch health.Status {
		case HealthStatusDegraded:
			statusCode = http.StatusOK // Still accepting traffic
		case HealthStatusUnhealthy:
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		// JSON response
		response := map[string]interface{}{
			"status":        health.Status,
			"shores":        len(health.Shores),
			"open_breakers": health.OpenBreakers,
		}
		json.NewEncoder(w).Encode(response)
	}
}
