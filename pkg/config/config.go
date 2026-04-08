package config

import (
	"fmt"
	"os"
)

type Config struct {
	ServiceName            string
	HTTPPort               string
	AdminPort              string
	LogLevel               string
	StorageBackend         string
	DatabaseURL            string
	OTLPEndpoint           string
	PyroscopeServerAddress string
}

func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:            os.Getenv("SERVICE_NAME"),
		HTTPPort:               envOrDefault("HTTP_PORT", "8080"),
		AdminPort:              envOrDefault("ADMIN_PORT", "9090"),
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
		StorageBackend:         envOrDefault("STORAGE_BACKEND", "memory"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		OTLPEndpoint:           os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
	}

	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("SERVICE_NAME environment variable is required")
	}

	if cfg.StorageBackend == "postgres" && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required when STORAGE_BACKEND is postgres")
	}

	return cfg, nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
