package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
	"github.com/tartarus-sandbox/tartarus/pkg/config"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
	"github.com/tartarus-sandbox/tartarus/pkg/phlegethon"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Starting Olympus API", "port", cfg.Port)

	// Adapters
	metrics := hermes.NewPrometheusMetrics()
	var queue acheron.Queue
	redisAddr := cfg.RedisAddress
	if redisAddr != "" {
		redisDB := 0
		if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
			if db, err := strconv.Atoi(dbStr); err == nil {
				redisDB = db
			}
		}
		redisKey := os.Getenv("REDIS_QUEUE_KEY")
		if redisKey == "" {
			redisKey = "tartarus:queue"
		}

		rq, err := acheron.NewRedisQueue(redisAddr, redisDB, redisKey, "", "", true, metrics, nil)
		if err != nil {
			logger.Error("Failed to initialize Redis queue", "error", err)
			os.Exit(1)
		}
		queue = rq
		logger.Info("Using Redis queue", "addr", redisAddr, "db", redisDB, "key", redisKey)
		logger.Info("Using Redis queue", "addr", redisAddr, "db", redisDB, "key", redisKey)
	} else {
		if os.Getenv("TARTARUS_ENV") == "production" {
			logger.Error("Redis queue is required in production mode (TARTARUS_ENV=production)")
			os.Exit(1)
		}
		queue = acheron.NewMemoryQueue()
		logger.Info("Using in-memory queue")
	}

	var registry hades.Registry
	if cfg.RedisAddress != "" {
		rr, err := hades.NewRedisRegistry(cfg.RedisAddress, cfg.RedisDB, cfg.RedisPass)
		if err != nil {
			logger.Error("Failed to initialize Redis registry", "error", err)
			os.Exit(1)
		}
		registry = rr
		logger.Info("Using Redis registry", "addr", cfg.RedisAddress)
		logger.Info("Using Redis registry", "addr", cfg.RedisAddress)
	} else {
		if os.Getenv("TARTARUS_ENV") == "production" {
			logger.Error("Redis registry is required in production mode (TARTARUS_ENV=production)")
			os.Exit(1)
		}
		memReg := hades.NewMemoryRegistry()
		registry = memReg
		logger.Info("Using in-memory registry")

		// Pre-populate a node for testing/dev
		memReg.UpdateHeartbeat(context.Background(), hades.HeartbeatPayload{
			Node: domain.NodeInfo{
				ID:      "test-node",
				Address: "127.0.0.1",
				Capacity: domain.ResourceCapacity{
					CPU: 8000,
					Mem: 8192,
				},
			},
			Load: domain.ResourceCapacity{
				CPU: 0,
				Mem: 0,
			},
			Time: time.Now(),
		})
	}

	var store erebus.Store
	if cfg.S3Endpoint != "" || cfg.S3Region != "" {
		// If S3 config is present, use S3Store
		s3Store, err := erebus.NewS3Store(context.Background(), cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.SnapshotPath)
		if err != nil {
			logger.Error("Failed to initialize S3 store", "error", err)
			os.Exit(1)
		}
		store = s3Store
		logger.Info("Using S3 store", "bucket", cfg.S3Bucket)
	} else {
		localStore, err := erebus.NewLocalStore(cfg.SnapshotPath)
		if err != nil {
			logger.Error("Failed to initialize local store", "error", err)
			os.Exit(1)
		}
		store = localStore
		logger.Info("Using local store", "path", cfg.SnapshotPath)
	}
	hermesLogger := hermes.NewSlogAdapter()
	ociBuilder := erebus.NewOCIBuilder(store, hermesLogger)

	// Nyx Manager
	nyxManager, err := nyx.NewLocalManager(store, ociBuilder, cfg.SnapshotPath, hermesLogger)
	if err != nil {
		logger.Error("Failed to initialize Nyx manager", "error", err)
		os.Exit(1)
	}

	scheduler := moirai.NewScheduler(cfg.SchedulerStrategy, hermesLogger)

	// Policy repository
	var policyRepo themis.Repository
	if cfg.RedisAddress != "" {
		rr, err := themis.NewRedisRepo(cfg.RedisAddress, cfg.RedisDB, cfg.RedisPass)
		if err != nil {
			logger.Error("Failed to initialize Redis policy repo", "error", err)
			os.Exit(1)
		}
		policyRepo = rr
		logger.Info("Using Redis policy repo", "addr", cfg.RedisAddress)
	} else {
		if os.Getenv("TARTARUS_ENV") == "production" {
			logger.Error("Redis policy repo is required in production mode (TARTARUS_ENV=production)")
			os.Exit(1)
		}
		policyRepo = themis.NewMemoryRepo()
		logger.Info("Using in-memory policy repo")
	}

	// Template Manager
	templateManager := olympus.NewMemoryTemplateManager()
	// Add default templates
	defaultTpl := &domain.TemplateSpec{
		ID:          "hello-world",
		Name:        "Hello World",
		Description: "A simple hello world template",
		BaseImage:   "/var/lib/tartarus/images/hello-world.ext4",
		KernelImage: "/var/lib/firecracker/vmlinux",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
	}
	templateManager.RegisterTemplate(context.Background(), defaultTpl)

	// Data Science Templates
	dsTemplates := []*domain.TemplateSpec{
		{
			ID:          "python-ds",
			Name:        "Python Data Science",
			Description: "Python environment with NumPy, Pandas, Scikit-learn preloaded",
			BaseImage:   "/var/lib/tartarus/images/python-ds.ext4",
			KernelImage: "/var/lib/firecracker/vmlinux",
			Resources: domain.ResourceSpec{
				CPU: 2000,
				Mem: 2048,
			},
			WarmupCommand: []string{"python3", "-c", "import numpy; import pandas; import sklearn"},
		},
		{
			ID:          "pytorch-ml",
			Name:        "PyTorch ML",
			Description: "PyTorch environment preloaded",
			BaseImage:   "/var/lib/tartarus/images/pytorch-ml.ext4",
			KernelImage: "/var/lib/firecracker/vmlinux",
			Resources: domain.ResourceSpec{
				CPU: 4000,
				Mem: 8192,
				GPU: domain.GPURequest{Count: 1, Type: "nvidia"},
			},
			WarmupCommand: []string{"python3", "-c", "import torch; import torchvision"},
		},
		{
			ID:          "r-analytics",
			Name:        "R Analytics",
			Description: "R environment with Tidyverse preloaded",
			BaseImage:   "/var/lib/tartarus/images/r-analytics.ext4",
			KernelImage: "/var/lib/firecracker/vmlinux",
			Resources: domain.ResourceSpec{
				CPU: 2000,
				Mem: 4096,
			},
			WarmupCommand: []string{"R", "-e", "library(dplyr); library(ggplot2)"},
		},
		{
			ID:          "julia-sci",
			Name:        "Julia Science",
			Description: "Julia environment preloaded",
			BaseImage:   "/var/lib/tartarus/images/julia-sci.ext4",
			KernelImage: "/var/lib/firecracker/vmlinux",
			Resources: domain.ResourceSpec{
				CPU: 2000,
				Mem: 4096,
			},
			WarmupCommand: []string{"julia", "-e", "using DataFrames"},
		},
	}
	for _, tpl := range dsTemplates {
		templateManager.RegisterTemplate(context.Background(), tpl)
	}

	// Add default policy for hello-world
	defaultPolicy := &domain.SandboxPolicy{
		ID:         "default-hello-world",
		TemplateID: "hello-world",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
		NetworkPolicy: domain.NetworkPolicyRef{
			ID:   "lockdown-no-net",
			Name: "No Internet",
		},
		Retention: domain.RetentionPolicy{
			MaxAge:      30 * time.Minute,
			KeepOutputs: true,
		},
	}
	policyRepo.UpsertPolicy(context.Background(), defaultPolicy)

	// Control Plane
	var control olympus.ControlPlane
	if redisAddr != "" {
		rdb := redis.NewClient(&redis.Options{
			Addr: redisAddr,
			// DB: redisDB,
		})
		control = olympus.NewRedisControlPlane(rdb)
		logger.Info("Using Redis control plane")
		logger.Info("Using Redis control plane")
	} else {
		if os.Getenv("TARTARUS_ENV") == "production" {
			logger.Error("Redis control plane is required in production mode (TARTARUS_ENV=production)")
			os.Exit(1)
		}
		control = &olympus.NoopControlPlane{}
		logger.Info("Using Noop control plane")
	}

	// Judges
	// Create audit sink for Aeacus
	auditSink := judges.NewLogAuditSink(hermesLogger)
	logger.Info("Initialized audit sink for Aeacus judge")

	aeacusJudge := judges.NewAeacusJudge(hermesLogger, auditSink)
	resourceJudge := judges.NewResourceJudge(policyRepo, hermesLogger)
	networkJudge := judges.NewNetworkJudge(cfg.AllowedNetworks, []netip.Prefix{}, hermesLogger)
	judgeChain := &judges.Chain{
		Pre: []judges.PreJudge{aeacusJudge, resourceJudge, networkJudge},
	}

	// Phlegethon Heat Classifier
	heatClassifier := phlegethon.NewHeatClassifier()
	// Add template hints if needed (could be loaded from config in the future)
	// heatClassifier.AddHint("gpu-training", phlegethon.HeatInferno)

	manager := &olympus.Manager{
		Queue:      queue,
		Hades:      registry,
		Policies:   policyRepo,
		Templates:  templateManager,
		Nyx:        nyxManager,
		Judges:     judgeChain,
		Scheduler:  scheduler,
		Phlegethon: heatClassifier,
		Control:    control,
		Metrics:    metrics,
		Logger:     hermesLogger,
	}

	// Reconcile state on startup
	logger.Info("Reconciling state from agents...")
	if err := manager.Reconcile(context.Background()); err != nil {
		// Log error but continue startup? Or fail?
		// If we fail, we might be in a crash loop if one agent is bad.
		// Reconcile already handles individual node errors by logging and continuing.
		// So if it returns error, it's likely a global failure (e.g. listing nodes failed).
		// Let's log error and continue, so API is at least available.
		logger.Error("Reconciliation failed", "error", err)
	} else {
		logger.Info("Reconciliation complete")
	}

	// Persephone Seasonal Scaler
	seasonalScaler := persephone.NewBasicSeasonalScaler()
	// Define default seasons
	seasonalScaler.DefineSeason(context.Background(), persephone.SeasonSpring)
	seasonalScaler.DefineSeason(context.Background(), persephone.SeasonSummer)
	seasonalScaler.DefineSeason(context.Background(), persephone.SeasonAutumn)
	seasonalScaler.DefineSeason(context.Background(), persephone.SeasonWinter)
	// Apply Spring as default for now
	seasonalScaler.ApplySeason(context.Background(), "spring")

	// Olympus Scaler
	scaler := olympus.NewScaler(seasonalScaler, registry, manager, hermesLogger, metrics)

	// Register seasons for automatic activation
	scaler.RegisterSeason(persephone.SeasonSpring)
	scaler.RegisterSeason(persephone.SeasonSummer)
	scaler.RegisterSeason(persephone.SeasonAutumn)
	scaler.RegisterSeason(persephone.SeasonWinter)

	go scaler.Run(context.Background())

	// Persephone API handlers
	persephoneHandlers := olympus.NewPersephoneHandlers(scaler)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req domain.SandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := manager.Submit(r.Context(), &req); err != nil {
			if errors.Is(err, olympus.ErrPolicyRejected) {
				logger.Warn("Request rejected by policy", "error", err)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			logger.Error("Failed to submit request", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "id": string(req.ID)})
	})

	mux.HandleFunc("/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		runs, err := manager.ListSandboxes(r.Context())
		if err != nil {
			logger.Error("Failed to list sandboxes", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(runs)
	})

	mux.HandleFunc("/sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		// /sandboxes/{id}
		// /sandboxes/{id}/snapshot
		// /sandboxes/{id}/snapshots
		// /sandboxes/{id}/snapshots/{snapID}
		// /sandboxes/{id}/exec

		path := r.URL.Path[len("/sandboxes/"):]
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}
		id := domain.SandboxID(parts[0])

		if len(parts) == 1 {
			// /sandboxes/{id}
			if r.Method == http.MethodDelete {
				if err := manager.KillSandbox(r.Context(), id); err != nil {
					if errors.Is(err, olympus.ErrSandboxNotFound) {
						http.Error(w, "Sandbox not found", http.StatusNotFound)
						return
					}
					logger.Error("Failed to kill sandbox", "id", id, "error", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"status": "killed", "id": string(id)})
				return
			}
			// GET /sandboxes/{id}
			if r.Method == http.MethodGet {
				run, err := manager.Hades.GetRun(r.Context(), id)
				if err != nil {
					if errors.Is(err, hades.ErrRunNotFound) {
						http.Error(w, "Sandbox not found", http.StatusNotFound)
						return
					}
					logger.Error("Failed to get sandbox", "id", id, "error", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(run)
				return
			}
			return
		}

		action := parts[1]
		switch action {
		case "snapshot":
			if r.Method == http.MethodPost {
				// Create Snapshot
				if err := manager.CreateSnapshot(r.Context(), id); err != nil {
					logger.Error("Failed to create snapshot", "id", id, "error", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(map[string]string{"status": "snapshot_requested", "id": string(id)})
				return
			}
		case "snapshots":
			if r.Method == http.MethodGet {
				// List Snapshots
				snaps, err := manager.ListSnapshots(r.Context(), id)
				if err != nil {
					logger.Error("Failed to list snapshots", "id", id, "error", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(snaps)
				return
			} else if r.Method == http.MethodDelete {
				// DELETE /sandboxes/{id}/snapshots/{snapID}
				if len(parts) < 3 {
					http.Error(w, "Missing snapshot ID", http.StatusBadRequest)
					return
				}
				snapID := domain.SnapshotID(parts[2])
				if err := manager.DeleteSnapshot(r.Context(), id, snapID); err != nil {
					logger.Error("Failed to delete snapshot", "id", id, "snapID", snapID, "error", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		case "exec":
			if r.Method == http.MethodPost {
				var req struct {
					Cmd []string `json:"cmd"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "Invalid request body", http.StatusBadRequest)
					return
				}
				if err := manager.Exec(r.Context(), id, req.Cmd); err != nil {
					logger.Error("Failed to exec", "id", id, "error", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				return
			}
		case "logs":
			// Handled by specific handler?
			// No, specific handler was /sandboxes/logs/
			// But /sandboxes/ prefix matches everything.
			// I need to be careful about overlapping handlers.
			// /sandboxes/ is a prefix match.
			// /sandboxes/logs/ is longer, so it should take precedence for /sandboxes/logs/...
			// But /sandboxes/{id}/logs is NOT /sandboxes/logs/...
			// My previous log handler was:
			// mux.HandleFunc("/sandboxes/logs/", ...) -> matches /sandboxes/logs/{id}
			// This handler matches /sandboxes/{id}/...
			// So if I request /sandboxes/{id}/logs, it comes here.
			// I should handle logs here too if I want /sandboxes/{id}/logs style.
			// But let's stick to existing style or redirect?
			// Existing: /sandboxes/logs/{id}
			// Let's keep existing.
		}
	})

	// Explicit handler for logs to make it cleaner?
	// But /sandboxes/ overlaps.
	// Let's use a specific prefix for logs or handle it in the generic handler.
	// Let's try to handle /sandboxes/{id}/logs by checking suffix.
	mux.HandleFunc("/sandboxes/logs/", func(w http.ResponseWriter, r *http.Request) {
		// /sandboxes/logs/{id}
		id := domain.SandboxID(r.URL.Path[len("/sandboxes/logs/"):])
		if id == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}

		follow := r.URL.Query().Get("follow") == "true"

		// Set headers for streaming
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if err := manager.StreamLogs(r.Context(), id, w, follow); err != nil {
			// Check if error is sandbox not found
			if errors.Is(err, olympus.ErrSandboxNotFound) {
				logger.Warn("Sandbox not found for log streaming", "id", id)
				// Can only send error if we haven't started writing
				// Since we set headers above, this will be logged but status may not change
				// if we already wrote something? No, headers are buffered until first write.
				// But we are using chunked encoding, so maybe.
				// Let's try to send error.
				http.Error(w, "Sandbox not found", http.StatusNotFound)
				return
			}
			logger.Error("Log streaming failed", "id", id, "error", err)
			// If streaming was already started, can't change status code
			// Error will be logged and connection closed
		}
	})

	mux.HandleFunc("/sandboxes/hibernate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := domain.SandboxID(r.URL.Path[len("/sandboxes/hibernate/"):])
		if id == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}

		if err := manager.HibernateSandbox(r.Context(), id); err != nil {
			if errors.Is(err, olympus.ErrSandboxNotFound) {
				http.Error(w, "Sandbox not found", http.StatusNotFound)
				return
			}
			logger.Error("Failed to hibernate sandbox", "id", id, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "hibernating", "id": string(id)})
	})

	mux.HandleFunc("/sandboxes/wake/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := domain.SandboxID(r.URL.Path[len("/sandboxes/wake/"):])
		if id == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}

		if err := manager.WakeSandbox(r.Context(), id); err != nil {
			if errors.Is(err, olympus.ErrSandboxNotFound) {
				http.Error(w, "Sandbox not found", http.StatusNotFound)
				return
			}
			logger.Error("Failed to wake sandbox", "id", id, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "waking", "id": string(id)})
	})

	mux.HandleFunc("/templates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tpls, err := templateManager.ListTemplates(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(tpls)
	})

	mux.HandleFunc("/policies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pols, err := policyRepo.ListPolicies(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(pols)
	})

	// Persephone endpoints
	mux.HandleFunc("/persephone/seasons", persephoneHandlers.HandleCreateSeason)
	mux.HandleFunc("/persephone/seasons/", func(w http.ResponseWriter, r *http.Request) {
		// Route to activate if path ends with /activate
		if strings.HasSuffix(r.URL.Path, "/activate") {
			persephoneHandlers.HandleActivateSeason(w, r)
		} else {
			persephoneHandlers.HandleListSeasons(w, r)
		}
	})
	mux.HandleFunc("/persephone/forecast", persephoneHandlers.HandleGetForecast)
	mux.HandleFunc("/persephone/recommendations", persephoneHandlers.HandleGetRecommendations)

	// Setup Cerberus gateway for authentication, authorization, and audit
	apiKey := os.Getenv("TARTARUS_API_KEY")

	// Authenticators
	var authenticators []cerberus.Authenticator

	// 1. API Key Authenticator
	if apiKey != "" {
		authenticators = append(authenticators, cerberus.NewSimpleAPIKeyAuthenticator(apiKey))
	}

	// 1.5 Signed API Key Authenticator (for rotated keys)
	// Uses SecretProvider to resolve signing keys
	// Chain: Env -> Vault -> KMS
	var secretProviders []cerberus.SecretProvider
	secretProviders = append(secretProviders, cerberus.NewEnvSecretProvider())

	if cfg.VaultAddress != "" {
		vaultConfig := cerberus.VaultConfig{
			Address:   cfg.VaultAddress,
			Token:     cfg.VaultToken,
			Namespace: cfg.VaultNamespace,
		}
		secretProviders = append(secretProviders, cerberus.NewRealVaultSecretProvider(vaultConfig))
		logger.Info("Enabled Vault secret provider", "address", cfg.VaultAddress)
	}

	if cfg.KMSRegion != "" {
		// KMS provider (actually SSM Parameter Store)
		kmsProvider, err := cerberus.NewKMSSecretProvider(context.Background(), cfg.KMSRegion)
		if err != nil {
			logger.Error("Failed to initialize KMS secret provider", "error", err)
			// Don't exit, just log error and continue without KMS
		} else {
			secretProviders = append(secretProviders, kmsProvider)
			logger.Info("Enabled KMS/SSM secret provider", "region", cfg.KMSRegion)
		}
	}

	compositeProvider := cerberus.NewCompositeSecretProvider(secretProviders...)
	authenticators = append(authenticators, cerberus.NewSignedAPIKeyAuthenticator(compositeProvider))

	// 2. OIDC Authenticator
	if cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" {
		oidcAuth, err := cerberus.NewOIDCAuthenticator(context.Background(), cfg.OIDCIssuerURL, cfg.OIDCClientID, "")
		if err != nil {
			logger.Error("Failed to initialize OIDC authenticator", "error", err)
			os.Exit(1)
		}
		authenticators = append(authenticators, oidcAuth)
		logger.Info("Enabled OIDC authentication", "issuer", cfg.OIDCIssuerURL)
	}

	// 3. mTLS Authenticator (for agent communication)
	if cfg.TLSClientAuth == "require-verify" && cfg.TLSCAFile != "" {
		// Load the CA pool for verifying client certificates
		caPool := x509.NewCertPool()
		caBytes, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			logger.Error("Failed to read CA file for mTLS", "error", err)
			os.Exit(1)
		}
		if !caPool.AppendCertsFromPEM(caBytes) {
			logger.Error("Failed to append CA certificates for mTLS")
			os.Exit(1)
		}
		mtlsAuth := cerberus.NewMTLSAuthenticator(caPool)
		authenticators = append(authenticators, mtlsAuth)
		logger.Info("Enabled mTLS authentication for agents")
	}

	var cerberusAuth cerberus.Authenticator
	if len(authenticators) == 0 {
		logger.Warn("Running in INSECURE mode: No authentication configured. All requests are allowed.")
		cerberusAuth = cerberus.NewSimpleAPIKeyAuthenticator("")
	} else if len(authenticators) == 1 {
		cerberusAuth = authenticators[0]
	} else {
		cerberusAuth = cerberus.NewMultiAuthenticator(authenticators...)
	}

	// Authorizer
	var cerberusAuthz cerberus.Authorizer
	if cfg.RBACPolicyPath != "" {
		loader := cerberus.NewRBACPolicyLoader()
		policies, err := loader.LoadPolicies(cfg.RBACPolicyPath)
		if err != nil {
			logger.Error("Failed to load RBAC policies", "path", cfg.RBACPolicyPath, "error", err)
			os.Exit(1)
		}
		cerberusAuthz = cerberus.NewRBACAuthorizer(policies)
		logger.Info("Enabled RBAC authorization", "policy_count", len(policies))
	} else {
		cerberusAuthz = cerberus.NewAllowAllAuthorizer()
		logger.Info("Using AllowAll authorizer (no RBAC policies configured)")
	}

	// Setup composite auditor (logs + metrics)
	cerberusAudit := cerberus.NewCompositeAuditor(
		cerberus.NewLogAuditor(logger),
		cerberus.NewMetricsAuditor(metrics),
	)

	// Create the three-headed gateway
	cerberusGateway := cerberus.NewGateway(cerberusAuth, cerberusAuthz, cerberusAudit)

	// Create credential extractor (supports both mTLS and bearer tokens)
	var credExtractor cerberus.CredentialExtractor
	if cfg.TLSClientAuth == "require-verify" {
		// Try mTLS first, then fall back to bearer token
		credExtractor = cerberus.NewCompositeCredentialExtractor(
			cerberus.NewMTLSExtractor(),
			cerberus.NewBearerTokenExtractor(),
		)
	} else {
		// Only bearer token auth
		credExtractor = cerberus.NewBearerTokenExtractor()
	}

	// Create HTTP middleware
	cerberusMiddleware := cerberus.NewHTTPMiddleware(
		cerberusGateway,
		credExtractor,
		cerberus.NewDefaultResourceMapper(),
	)

	// Wrap the mux with Cerberus middleware
	var handler http.Handler = mux
	if len(authenticators) > 0 {
		handler = cerberusMiddleware.Wrap(mux)
	}

	// TLS Configuration
	var tlsConfig *tls.Config
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		var clientAuth tls.ClientAuthType
		if cfg.TLSClientAuth != "none" && cfg.TLSCAFile != "" {
			switch cfg.TLSClientAuth {
			case "request":
				clientAuth = tls.RequestClientCert
			case "require":
				clientAuth = tls.RequireAnyClientCert
			case "verify-if-given":
				clientAuth = tls.VerifyClientCertIfGiven
			case "require-verify":
				clientAuth = tls.RequireAndVerifyClientCert
			default:
				logger.Warn("Unknown TLS client auth mode, defaulting to NoClientCert", "mode", cfg.TLSClientAuth)
				clientAuth = tls.NoClientCert
			}
			logger.Info("Enabled mTLS with automated rotation", "client_auth", cfg.TLSClientAuth)
		}

		// Use CertWatcher for automated rotation
		watcher, err := cerberus.NewCertWatcher(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSCAFile, clientAuth, logger)
		if err != nil {
			logger.Error("Failed to initialize certificate watcher", "error", err)
			os.Exit(1)
		}

		// Start watcher in background
		go watcher.Start(context.Background(), 1*time.Minute)

		tlsConfig = watcher.TLSConfig()
	}

	srv := &http.Server{
		Addr:      ":" + cfg.Port,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}

	go func() {
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			logger.Info("Starting HTTPS server", "port", cfg.Port)
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				logger.Error("Server failed", "error", err)
			}
		} else {
			logger.Info("Starting HTTP server", "port", cfg.Port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Server failed", "error", err)
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}
	logger.Info("Server exited")
}
