package olympus

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		envKey         string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Insecure Mode (No Env Key)",
			envKey:         "",
			authHeader:     "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Secure Mode - Valid Key",
			envKey:         "secret-key",
			authHeader:     "Bearer secret-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Secure Mode - Missing Header",
			envKey:         "secret-key",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Secure Mode - Invalid Header Format",
			envKey:         "secret-key",
			authHeader:     "Basic secret-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Secure Mode - Wrong Key",
			envKey:         "secret-key",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv("TARTARUS_API_KEY", tt.envKey)
			} else {
				os.Unsetenv("TARTARUS_API_KEY")
			}

			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			handler := AuthMiddleware(logger, nextHandler)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}
