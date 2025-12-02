package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tartarus-sandbox/tartarus/pkg/charon"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Config represents the Charon proxy configuration.
type Config struct {
	ListenAddr string              `json:"listen_addr"`
	Ferry      *charon.FerryConfig `json:"ferry"`
	Shores     []*charon.Shore     `json:"shores"`
}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "charon.json", "Path to configuration file")
	listenAddr := flag.String("listen", ":8000", "Address to listen on")
	flag.Parse()

	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	config, err := loadConfig(*configFile, *listenAddr)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize metrics
	metrics := hermes.NewPrometheusMetrics()
	config.Ferry.Metrics = metrics

	// Create ferry
	ferry, err := charon.NewBoatFerry(config.Ferry)
	if err != nil {
		slog.Error("Failed to create ferry", "error", err)
		os.Exit(1)
	}

	// Register shores
	for _, shore := range config.Shores {
		if err := ferry.RegisterShore(shore); err != nil {
			slog.Error("Failed to register shore", "shore_id", shore.ID, "error", err)
			os.Exit(1)
		}
		slog.Info("Registered shore", "shore_id", shore.ID, "address", shore.Address)
	}

	// Start ferry (health checking, etc.)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ferry.Start(ctx)
	slog.Info("Ferry started, health checking enabled")

	// Create HTTP server
	mux := http.NewServeMux()

	// Health check endpoint
	middleware := charon.NewFerryMiddleware(ferry)
	mux.HandleFunc("/health", middleware.HealthHandler())

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Proxy all other requests
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp, err := ferry.Cross(r.Context(), r)
		if err != nil {
			httpErr := charon.ToHTTPError(err)
			http.Error(w, httpErr.Message, httpErr.HTTPStatusCode())
			return
		}

		// Copy response
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)

		if resp.Body != nil {
			defer resp.Body.Close()
			// Body is already written by the reverse proxy
		}
	})

	server := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Charon proxy listening", "address", config.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	slog.Info("Shutdown signal received, gracefully shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown error", "error", err)
	}

	if err := ferry.Close(); err != nil {
		slog.Error("Ferry close error", "error", err)
	}

	slog.Info("Charon proxy stopped")
}

// loadConfig loads configuration from file or uses defaults.
func loadConfig(configFile, listenAddr string) (*Config, error) {
	// Try to load from file
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		var config Config
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		return &config, nil
	}

	// Use defaults with environment variables
	slog.Info("Config file not found, using defaults and environment variables")

	config := &Config{
		ListenAddr: listenAddr,
		Ferry:      charon.DefaultFerryConfig(),
		Shores:     make([]*charon.Shore, 0),
	}

	// Load shores from environment
	// Format: CHARON_SHORE_<ID>=<address>
	// Example: CHARON_SHORE_OLYMPUS1=http://localhost:8080
	for _, env := range os.Environ() {
		if len(env) > 13 && env[:13] == "CHARON_SHORE_" {
			// Parse env var
			parts := parseEnvVar(env)
			if len(parts) == 2 {
				shoreID := parts[0][13:] // Remove "CHARON_SHORE_" prefix
				address := parts[1]

				shore := &charon.Shore{
					ID:          shoreID,
					Address:     address,
					Weight:      1,
					HealthCheck: charon.DefaultHealthCheck(),
				}
				config.Shores = append(config.Shores, shore)
			}
		}
	}

	// If no shores configured, add a default one
	if len(config.Shores) == 0 {
		olympusAddr := os.Getenv("OLYMPUS_ADDR")
		if olympusAddr == "" {
			olympusAddr = "http://localhost:8080"
		}

		config.Shores = append(config.Shores, &charon.Shore{
			ID:          "olympus-default",
			Address:     olympusAddr,
			Weight:      1,
			HealthCheck: charon.DefaultHealthCheck(),
		})
	}

	// Configure rate limiting from environment
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		config.Ferry.RateLimiting.RedisAddr = redisAddr
	}

	return config, nil
}

// parseEnvVar parses an environment variable in the format KEY=VALUE.
func parseEnvVar(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}
