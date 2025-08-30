package config

import (
	"os"
	"reflect"
	"testing"
)

func unsetEnv(keys ...string) func() {
	prev := make(map[string]string)
	for _, k := range keys {
		prev[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for k, v := range prev {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	restore := unsetEnv("PORT", "DATABASE_URL", "OTEL_EXPORTER_OTLP_ENDPOINT", "NATS_URL", "KAFKA_BROKERS")
	defer restore()

	cfg := Load()
	if cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://postgres:postgres@localhost:5432/ops?sslmode=disable" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.OtlpEndpoint != "otelcol:4317" {
		t.Fatalf("unexpected OTLP endpoint: %s", cfg.OtlpEndpoint)
	}
	if cfg.NatsURL != "nats://localhost:4222" {
		t.Fatalf("unexpected nats url: %s", cfg.NatsURL)
	}
	expBrokers := []string{"localhost:9092"}
	if !reflect.DeepEqual(cfg.KafkaBrokers, expBrokers) {
		t.Fatalf("unexpected brokers: %#v", cfg.KafkaBrokers)
	}
}

func TestLoadOverrides(t *testing.T) {
	restore := unsetEnv("PORT", "DATABASE_URL", "OTEL_EXPORTER_OTLP_ENDPOINT", "NATS_URL", "KAFKA_BROKERS")
	defer restore()

	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/app")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel:55680")
	os.Setenv("NATS_URL", "nats://nats:4222")
	os.Setenv("KAFKA_BROKERS", "k1:9092,k2:9093")

	cfg := Load()
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/app" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.OtlpEndpoint != "otel:55680" {
		t.Fatalf("unexpected OTLP endpoint: %s", cfg.OtlpEndpoint)
	}
	if cfg.NatsURL != "nats://nats:4222" {
		t.Fatalf("unexpected nats url: %s", cfg.NatsURL)
	}
	expBrokers := []string{"k1:9092", "k2:9093"}
	if !reflect.DeepEqual(cfg.KafkaBrokers, expBrokers) {
		t.Fatalf("unexpected brokers: %#v", cfg.KafkaBrokers)
	}
}
