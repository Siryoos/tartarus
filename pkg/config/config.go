package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port         string
	Region       string
	SnapshotPath string
	LogLevel     string

	SchedulerStrategy string

	RedisAddress string
	RedisDB      int
	RedisPass    string

	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string

	AllowedNetworks []string

	// Phase 4 feature flags (disabled by default for v1.0 stability)
	EnableHypnos bool
	// Thanatos (Graceful Termination) is always enabled

	// Cerberus Auth Config
	OIDCClientID   string
	OIDCIssuerURL  string
	RBACPolicyPath string
	TLSCertFile    string
	TLSKeyFile     string
	TLSClientAuth  string // "none", "request", "require", "verify-if-given", "require-verify"
	TLSCAFile      string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		Region:       getEnv("REGION", "local"),
		SnapshotPath: getEnv("SNAPSHOT_PATH", "/tmp/tartarus/snapshots"),
		LogLevel:     getEnv("LOG_LEVEL", "INFO"),

		SchedulerStrategy: getEnv("SCHEDULER_STRATEGY", "least-loaded"),

		RedisAddress: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:      GetEnvInt("REDIS_DB", 0),
		RedisPass:    getEnv("REDIS_PASSWORD", ""),

		S3Endpoint:  getEnv("S3_ENDPOINT", ""),
		S3Region:    getEnv("S3_REGION", "us-east-1"),
		S3Bucket:    getEnv("S3_BUCKET", "tartarus-snapshots"),
		S3AccessKey: getEnv("AWS_ACCESS_KEY_ID", ""),
		S3SecretKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),

		AllowedNetworks: strings.Split(getEnv("ALLOWED_NETWORKS", "no-net,lockdown"), ","),

		// Phase 4 feature flags (disabled by default for v1.0 stability)
		EnableHypnos: GetEnvBool("ENABLE_HYPNOS", false),
		// Thanatos is now always enabled - no feature flag needed

		// Cerberus Auth Config
		OIDCClientID:   getEnv("OIDC_CLIENT_ID", ""),
		OIDCIssuerURL:  getEnv("OIDC_ISSUER_URL", ""),
		RBACPolicyPath: getEnv("RBAC_POLICY_PATH", ""),
		TLSCertFile:    getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:     getEnv("TLS_KEY_FILE", ""),
		TLSClientAuth:  getEnv("TLS_CLIENT_AUTH", "none"),
		TLSCAFile:      getEnv("TLS_CA_FILE", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		lowerValue := strings.ToLower(value)
		return lowerValue == "true" || lowerValue == "1" || lowerValue == "yes"
	}
	return fallback
}

// GetEnv returns an environment variable or a fallback value (exported for external use).
func GetEnv(key, fallback string) string {
	return getEnv(key, fallback)
}
