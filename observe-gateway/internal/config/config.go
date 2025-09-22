package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration loaded from YAML.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Auth        AuthConfig        `yaml:"auth"`
	RateLimiter RateLimiterConfig `yaml:"rate_limiter"`
	Cache       CacheConfig       `yaml:"cache"`
	Audit       AuditConfig       `yaml:"audit"`
	Backends    BackendConfig     `yaml:"backends"`
}

// ServerConfig controls HTTP server settings.
type ServerConfig struct {
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// AuthConfig configures JWT based authentication.
type AuthConfig struct {
	Enabled     bool          `yaml:"enabled"`
	JWKSURL     string        `yaml:"jwks_url"`
	Audience    []string      `yaml:"audience"`
	Issuer      string        `yaml:"issuer"`
	TenantClaim string        `yaml:"tenant_claim"`
	UserClaim   string        `yaml:"user_claim"`
	CacheTTL    time.Duration `yaml:"cache_ttl"`
	InsecureTLs bool          `yaml:"insecure_tls"`
}

// RateLimiterConfig defines per-tenant rate limiting behaviour.
type RateLimiterConfig struct {
	Enabled            bool          `yaml:"enabled"`
	RequestsPerSecond  float64       `yaml:"requests_per_second"`
	Burst              int           `yaml:"burst"`
	Window             time.Duration `yaml:"window"`
	RedisAddr          string        `yaml:"redis_addr"`
	RedisUsername      string        `yaml:"redis_username"`
	RedisPassword      string        `yaml:"redis_password"`
	RedisDB            int           `yaml:"redis_db"`
	RedisTLSInsecure   bool          `yaml:"redis_tls_insecure"`
	RedisTLSCA         string        `yaml:"redis_tls_ca"`
	RedisTLSSkipVerify bool          `yaml:"redis_tls_skip_verify"`
}

// CacheConfig configures ristretto caching behaviour.
type CacheConfig struct {
	Enabled     bool          `yaml:"enabled"`
	NumCounters int64         `yaml:"num_counters"`
	MaxCost     int64         `yaml:"max_cost"`
	BufferItems int64         `yaml:"buffer_items"`
	TTL         time.Duration `yaml:"ttl"`
}

// AuditConfig configures request auditing.
type AuditConfig struct {
	Enabled bool `yaml:"enabled"`
}

// BackendConfig bundles configuration for upstream services.
type BackendConfig struct {
	OpenObserve OpenObserveConfig `yaml:"openobserve"`
	Fallback    FallbackConfig    `yaml:"fallback"`
	Metadata    MetadataConfig    `yaml:"metadata"`
}

// OpenObserveConfig defines endpoints for OpenObserve services.
type OpenObserveConfig struct {
	BaseURL             string        `yaml:"base_url"`
	Org                 string        `yaml:"org"`
	APIKey              string        `yaml:"api_key"`
	Timeout             time.Duration `yaml:"timeout"`
	PromQueryEndpoint   string        `yaml:"prom_query_endpoint"`
	PromRangeEndpoint   string        `yaml:"prom_range_endpoint"`
	LogSearchEndpoint   string        `yaml:"log_search_endpoint"`
	TraceSearchEndpoint string        `yaml:"trace_search_endpoint"`
	LogTable            string        `yaml:"log_table"`
	TraceTable          string        `yaml:"trace_table"`
}

// FallbackConfig defines configuration for VM/Mimir PromQL fallback.
type FallbackConfig struct {
	Enabled       bool          `yaml:"enabled"`
	BaseURL       string        `yaml:"base_url"`
	APIKey        string        `yaml:"api_key"`
	Timeout       time.Duration `yaml:"timeout"`
	QueryEndpoint string        `yaml:"query_endpoint"`
	RangeEndpoint string        `yaml:"range_endpoint"`
}

// MetadataConfig describes PostgreSQL metadata lookup configuration.
type MetadataConfig struct {
	Enabled           bool          `yaml:"enabled"`
	DSN               string        `yaml:"dsn"`
	MaxConnections    int32         `yaml:"max_connections"`
	MaxConnIdleTime   time.Duration `yaml:"max_conn_idle_time"`
	TenantLookupQuery string        `yaml:"tenant_lookup_query"`
}

// Load reads configuration from the supplied path or returns defaults.
func Load(path string) (Config, error) {
	cfg := defaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Address:      ":8080",
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Auth: AuthConfig{
			Enabled:     false,
			TenantClaim: "tenant",
			UserClaim:   "sub",
			CacheTTL:    time.Hour,
		},
		RateLimiter: RateLimiterConfig{
			Enabled:           false,
			RequestsPerSecond: 10,
			Burst:             20,
			Window:            time.Minute,
		},
		Cache: CacheConfig{
			Enabled:     false,
			NumCounters: 1e4,
			MaxCost:     1 << 28,
			BufferItems: 64,
			TTL:         time.Minute,
		},
		Audit: AuditConfig{Enabled: true},
		Backends: BackendConfig{
			OpenObserve: OpenObserveConfig{
				BaseURL:             "http://localhost:5080",
				Org:                 "default",
				Timeout:             30 * time.Second,
				PromQueryEndpoint:   "/api/%s/promql/query",
				PromRangeEndpoint:   "/api/%s/promql/query_range",
				LogSearchEndpoint:   "/api/%s/_search",
				TraceSearchEndpoint: "/api/%s/traces",
				LogTable:            "logs",
				TraceTable:          "traces",
			},
		},
	}
}
