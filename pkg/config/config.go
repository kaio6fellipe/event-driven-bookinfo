// Package config loads service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for a service.
type Config struct {
	ServiceName            string
	HTTPPort               string
	AdminPort              string
	LogLevel               string
	StorageBackend         string
	DatabaseURL            string
	RunMigrations          bool
	OTLPEndpoint           string
	PyroscopeServerAddress string
	RedisURL               string

	// Ingestion service configuration
	GatewayURL         string
	PollInterval       time.Duration
	SearchQueries      []string
	MaxResultsPerQuery int

	// Kafka producer configuration (used by ingestion service)
	KafkaBrokers string
	KafkaTopic   string
}

// Load reads configuration from environment variables and returns a Config.
// Returns an error if required variables are missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:            os.Getenv("SERVICE_NAME"),
		HTTPPort:               envOrDefault("HTTP_PORT", "8080"),
		AdminPort:              envOrDefault("ADMIN_PORT", "9090"),
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
		StorageBackend:         envOrDefault("STORAGE_BACKEND", "memory"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		RunMigrations:          envOrDefault("RUN_MIGRATIONS", "true") == "true",
		OTLPEndpoint:           os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		RedisURL:               os.Getenv("REDIS_URL"),

		GatewayURL:         envOrDefault("GATEWAY_URL", "http://localhost:8080"),
		PollInterval:       parseDuration(envOrDefault("POLL_INTERVAL", "5m")),
		SearchQueries:      parseCSV(envOrDefault("SEARCH_QUERIES", "programming,golang")),
		MaxResultsPerQuery: parseInt(envOrDefault("MAX_RESULTS_PER_QUERY", "10")),

		KafkaBrokers: os.Getenv("KAFKA_BROKERS"),
		KafkaTopic:   envOrDefault("KAFKA_TOPIC", "raw_books_details"),
	}

	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("SERVICE_NAME environment variable is required")
	}

	if cfg.StorageBackend == "postgres" && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required when STORAGE_BACKEND is postgres")
	}

	return cfg, nil
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 10
	}
	return n
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
