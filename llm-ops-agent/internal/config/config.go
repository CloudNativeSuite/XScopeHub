package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port         int
	DatabaseURL  string
	OtlpEndpoint string
	NatsURL      string
	KafkaBrokers []string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	port, _ := strconv.Atoi(getenv("PORT", "8080"))
	return Config{
		Port:         port,
		DatabaseURL:  getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ops?sslmode=disable"),
		OtlpEndpoint: getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otelcol:4317"),
		NatsURL:      getenv("NATS_URL", "nats://localhost:4222"),
		KafkaBrokers: strings.Split(getenv("KAFKA_BROKERS", "localhost:9092"), ","),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
