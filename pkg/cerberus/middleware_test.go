package cerberus

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestBearerTokenExtractor(t *testing.T) {
	extractor := NewBearerTokenExtractor()

	tests := []struct {
		name       string
		authHeader string
		wantErr    bool
		wantSecret string
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer secret-key",
			wantErr:    false,
			wantSecret: "secret-key",
		},
		{
			name:       "missing authorization header",
			authHeader: "",
			wantErr:    true,
		},
		{
			name:       "invalid format - no bearer",
			authHeader: "Basic secret-key",
			wantErr:    true,
		},
		{
			name:       "invalid format - no space",
			authHeader: "Bearersecret-key",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			creds, err := extractor.Extract(req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			apiKeyCred, ok := creds.(*APIKeyCredential)
			if !ok {
				t.Errorf("expected APIKeyCredential, got %T", creds)
				return
			}

			if apiKeyCred.Secret != tt.wantSecret {
				t.Errorf("got secret %s, want %s", apiKeyCred.Secret, tt.wantSecret)
			}
		})
	}
}

func TestDefaultResourceMapper(t *testing.T) {
	mapper := NewDefaultResourceMapper()

	tests := []struct {
		name           string
		method         string
		path           string
		wantAction     Action
		wantResource   ResourceType
		wantResourceID string
	}{
		{
			name:           "GET /sandboxes",
			method:         "GET",
			path:           "/sandboxes",
			wantAction:     ActionRead,
			wantResource:   ResourceTypeSandbox,
			wantResourceID: "",
		},
		{
			name:           "POST /sandboxes",
			method:         "POST",
			path:           "/sandboxes",
			wantAction:     ActionCreate,
			wantResource:   ResourceTypeSandbox,
			wantResourceID: "",
		},
		{
			name:           "DELETE /sandboxes/123",
			method:         "DELETE",
			path:           "/sandboxes/123",
			wantAction:     ActionDelete,
			wantResource:   ResourceTypeSandbox,
			wantResourceID: "123",
		},
		{
			name:           "GET /templates",
			method:         "GET",
			path:           "/templates",
			wantAction:     ActionRead,
			wantResource:   ResourceTypeTemplate,
			wantResourceID: "",
		},
		{
			name:           "PUT /policies",
			method:         "PUT",
			path:           "/policies",
			wantAction:     ActionUpdate,
			wantResource:   ResourceTypePolicy,
			wantResourceID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)

			action, resource, err := mapper.MapRequest(req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if action != tt.wantAction {
				t.Errorf("got action %s, want %s", action, tt.wantAction)
			}

			if resource.Type != tt.wantResource {
				t.Errorf("got resource type %s, want %s", resource.Type, tt.wantResource)
			}

			if resource.ID != tt.wantResourceID {
				t.Errorf("got resource ID %s, want %s", resource.ID, tt.wantResourceID)
			}
		})
	}
}

func TestHTTPMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup gateway
	auth := NewSimpleAPIKeyAuthenticator("valid-key")
	authz := NewAllowAllAuthorizer()
	audit := NewLogAuditor(logger)
	gateway := NewGateway(auth, authz, audit)

	// Setup middleware
	extractor := NewBearerTokenExtractor()
	mapper := NewDefaultResourceMapper()
	middleware := NewHTTPMiddleware(gateway, extractor, mapper)

	// Create a test handler
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that identity is in context
		identity, ok := GetIdentity(r.Context())
		if !ok {
			t.Error("identity not found in context")
			return
		}

		if identity.ID != "api-key-user" {
			t.Errorf("got identity ID %s, want api-key-user", identity.ID)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := middleware.Wrap(nextHandler)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid authentication",
			authHeader:     "Bearer valid-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid API key",
			authHeader:     "Bearer invalid-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid header format",
			authHeader:     "Basic valid-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/sandboxes", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.expectedStatus)
			}
		})
	}
}

func TestHTTPMiddleware_Authorization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup gateway with deny-all authorizer
	auth := NewSimpleAPIKeyAuthenticator("valid-key")
	authz := NewDenyAllAuthorizer() // Deny all requests
	audit := NewLogAuditor(logger)
	gateway := NewGateway(auth, authz, audit)

	extractor := NewBearerTokenExtractor()
	mapper := NewDefaultResourceMapper()
	middleware := NewHTTPMiddleware(gateway, extractor, mapper)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when authorization fails")
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.Wrap(nextHandler)

	req := httptest.NewRequest("POST", "/sandboxes", nil)
	req.Header.Set("Authorization", "Bearer valid-key")

	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGetIdentity(t *testing.T) {
	identity := &Identity{
		ID:   "test-user",
		Type: IdentityTypeUser,
	}

	ctx := context.WithValue(context.Background(), IdentityContextKey, identity)

	retrieved, ok := GetIdentity(ctx)
	if !ok {
		t.Error("identity not found in context")
	}

	if retrieved.ID != identity.ID {
		t.Errorf("got identity ID %s, want %s", retrieved.ID, identity.ID)
	}

	// Test with empty context
	_, ok = GetIdentity(context.Background())
	if ok {
		t.Error("expected identity not found, but got one")
	}
}
