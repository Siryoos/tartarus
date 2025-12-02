package cerberus

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestRealVaultSecretProvider(t *testing.T) {
	config := VaultConfig{
		Address: "http://localhost:8200",
		Token:   "test-token",
	}
	p := NewRealVaultSecretProvider(config)

	// Mock HTTP client
	p.client.Transport = &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Check URL and headers
			if req.Header.Get("X-Vault-Token") != "test-token" {
				return &http.Response{StatusCode: http.StatusForbidden}, nil
			}

			// Mock response for vault:secret/myapp:api_key
			// URL should be http://localhost:8200/v1/secret/myapp
			if req.URL.Path == "/v1/secret/myapp" {
				json := `{"data": {"data": {"api_key": "secret-value"}, "metadata": {}}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(json)),
				}, nil
			}

			return &http.Response{StatusCode: http.StatusNotFound}, nil
		},
	}

	ctx := context.Background()

	// Test Success
	got, err := p.Resolve(ctx, "vault:secret/myapp:api_key")
	if err != nil {
		t.Errorf("Resolve() error = %v", err)
	}
	if got != "secret-value" {
		t.Errorf("Resolve() got = %v, want %v", got, "secret-value")
	}

	// Test Cache Hit (modify mock to fail, should still succeed)
	p.client.Transport = &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusInternalServerError}, nil
		},
	}
	got, err = p.Resolve(ctx, "vault:secret/myapp:api_key")
	if err != nil {
		t.Errorf("Resolve() cached error = %v", err)
	}
	if got != "secret-value" {
		t.Errorf("Resolve() cached got = %v, want %v", got, "secret-value")
	}

	// Test Invalid Format
	_, err = p.Resolve(ctx, "invalid:ref")
	if err == nil {
		t.Error("Resolve() expected error for invalid format")
	}
}
