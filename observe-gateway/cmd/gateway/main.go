package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/xscopehub/observe-gateway/internal/audit"
	"github.com/xscopehub/observe-gateway/internal/auth"
	"github.com/xscopehub/observe-gateway/internal/backend"
	"github.com/xscopehub/observe-gateway/internal/cache"
	"github.com/xscopehub/observe-gateway/internal/config"
	"github.com/xscopehub/observe-gateway/internal/limiter"
	"github.com/xscopehub/observe-gateway/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authenticator, err := auth.New(cfg.Auth)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}

	redisClient, err := buildRedisClient(cfg.RateLimiter)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	cacheCfg := cache.Config{
		Enabled:     cfg.Cache.Enabled,
		NumCounters: cfg.Cache.NumCounters,
		MaxCost:     cfg.Cache.MaxCost,
		BufferItems: cfg.Cache.BufferItems,
		TTL:         cfg.Cache.TTL,
	}
	cacheStore, err := cache.New(cacheCfg)
	if err != nil {
		log.Fatalf("init cache: %v", err)
	}

	limiterCfg := limiter.Config{
		Enabled:           cfg.RateLimiter.Enabled,
		RequestsPerSecond: cfg.RateLimiter.RequestsPerSecond,
		Burst:             cfg.RateLimiter.Burst,
		Window:            cfg.RateLimiter.Window,
		Redis:             redisClient,
	}
	limit := limiter.New(limiterCfg)

	backendClient, err := backend.New(ctx, cfg.Backends)
	if err != nil {
		log.Fatalf("init backend: %v", err)
	}
	defer backendClient.Close()

	auditLogger := audit.New(cfg.Audit.Enabled, os.Stdout)

	srv := server.New(cfg, authenticator, backendClient, cacheStore, limit, auditLogger)

	log.Printf("query gateway listening on %s", cfg.Server.Address)
	if err := srv.Run(ctx); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server stopped: %v", err)
		}
	}
}

func buildRedisClient(cfg config.RateLimiterConfig) (redis.UniversalClient, error) {
	if !cfg.Enabled || cfg.RedisAddr == "" {
		return nil, nil
	}

	options := &redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	if cfg.RedisTLSInsecure || cfg.RedisTLSSkipVerify || cfg.RedisTLSCA != "" {
		tlsConfig := &tls.Config{InsecureSkipVerify: cfg.RedisTLSSkipVerify}
		if cfg.RedisTLSCA != "" {
			ca, err := os.ReadFile(cfg.RedisTLSCA)
			if err != nil {
				return nil, err
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(ca) {
				return nil, errors.New("failed to append redis tls ca")
			}
			tlsConfig.RootCAs = pool
		}
		options.TLSConfig = tlsConfig
	}

	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
