package cache

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto"
)

// Cache wraps a ristretto cache with feature toggle awareness.
type Cache struct {
	enabled bool
	ttl     time.Duration
	store   *ristretto.Cache
}

// Config captures cache construction parameters.
type Config struct {
	Enabled     bool
	NumCounters int64
	MaxCost     int64
	BufferItems int64
	TTL         time.Duration
}

// New creates a Cache instance according to the configuration.
func New(cfg Config) (*Cache, error) {
	if !cfg.Enabled {
		return &Cache{enabled: false}, nil
	}

	rc, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: int64OrDefault(cfg.BufferItems, 64),
	})
	if err != nil {
		return nil, err
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}

	return &Cache{enabled: true, ttl: ttl, store: rc}, nil
}

// Get returns cached bytes for the key, if available.
func (c *Cache) Get(_ context.Context, key string) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}
	if v, ok := c.store.Get(key); ok {
		if b, ok := v.([]byte); ok {
			return b, true
		}
	}
	return nil, false
}

// Set stores the payload in cache.
func (c *Cache) Set(_ context.Context, key string, val []byte, cost int64) {
	if !c.enabled {
		return
	}
	if cost <= 0 {
		cost = int64(len(val))
	}
	c.store.SetWithTTL(key, val, cost, c.ttl)
}

func int64OrDefault(v, def int64) int64 {
	if v <= 0 {
		return def
	}
	return v
}
