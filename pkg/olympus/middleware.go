package olympus

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// AuthMiddleware enforces API key authentication.
// It checks the TARTARUS_API_KEY environment variable.
// If the variable is not set, it logs a warning and allows all requests (INSECURE mode).
// If set, it requires the Authorization header to contain "Bearer <key>".
//
// Deprecated: Use pkg/cerberus.HTTPMiddleware instead for more comprehensive
// authentication, authorization, and audit capabilities. This function is
// maintained for backward compatibility only.
func AuthMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	apiKey := os.Getenv("TARTARUS_API_KEY")
	if apiKey == "" {
		logger.Warn("Running in INSECURE mode: TARTARUS_API_KEY is not set. All requests are allowed.")
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "Unauthorized: Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		// ConstantTimeCompare returns 1 if the two slices are equal, 0 otherwise.
		if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
