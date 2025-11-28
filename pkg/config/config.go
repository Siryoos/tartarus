package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port         string
	Region       string
	SnapshotPath string
	LogLevel     string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		Region:       getEnv("REGION", "local"),
		SnapshotPath: getEnv("SNAPSHOT_PATH", "/tmp/tartarus/snapshots"),
		LogLevel:     getEnv("LOG_LEVEL", "INFO"),
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
