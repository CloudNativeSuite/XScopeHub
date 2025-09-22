package limiter

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// ErrRateLimited indicates tenant exceeded rate limits.
var ErrRateLimited = errors.New("rate limit exceeded")

// Limiter enforces per-tenant limits using local token buckets and optional Redis sliding window.
type Limiter struct {
	enabled bool

	rps    float64
	burst  int
	window time.Duration

	localMu sync.Mutex
	local   map[string]*rate.Limiter

	redis redis.UniversalClient
}

// Config contains parameters for limiter construction.
type Config struct {
	Enabled           bool
	RequestsPerSecond float64
	Burst             int
	Window            time.Duration
	Redis             redis.UniversalClient
}

// New creates a Limiter from the supplied configuration.
func New(cfg Config) *Limiter {
	if !cfg.Enabled {
		return &Limiter{enabled: false}
	}

	if cfg.Burst <= 0 {
		cfg.Burst = int(cfg.RequestsPerSecond * 2)
		if cfg.Burst < 1 {
			cfg.Burst = 1
		}
	}

	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}

	return &Limiter{
		enabled: true,
		rps:     cfg.RequestsPerSecond,
		burst:   cfg.Burst,
		window:  cfg.Window,
		local:   make(map[string]*rate.Limiter),
		redis:   cfg.Redis,
	}
}

// Allow verifies whether the tenant may perform the next action.
func (l *Limiter) Allow(ctx context.Context, tenant string) error {
	if !l.enabled || tenant == "" {
		return nil
	}

	if !l.allowLocal(tenant) {
		return ErrRateLimited
	}

	if l.redis != nil {
		allowed, err := l.allowRedis(ctx, tenant)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrRateLimited
		}
	}

	return nil
}

func (l *Limiter) allowLocal(tenant string) bool {
	l.localMu.Lock()
	limiter := l.local[tenant]
	if limiter == nil {
		limit := rate.Inf
		if l.rps > 0 {
			limit = rate.Limit(l.rps)
		}
		limiter = rate.NewLimiter(limit, l.burst)
		l.local[tenant] = limiter
	}
	l.localMu.Unlock()

	if limiter == nil {
		return true
	}
	return limiter.Allow()
}

var redisScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count >= limit then
  return 0
end
redis.call('ZADD', key, now, now)
redis.call('PEXPIRE', key, window)
return 1
`)

func (l *Limiter) allowRedis(ctx context.Context, tenant string) (bool, error) {
	if l.redis == nil {
		return true, nil
	}

	limit := l.burst
	if limit <= 0 {
		limit = 1
	}

	now := time.Now().UnixMilli()
	window := l.window.Milliseconds()
	if window <= 0 {
		window = time.Minute.Milliseconds()
	}

	res, err := redisScript.Run(ctx, l.redis, []string{"rate:" + tenant}, now, window, limit).Int()
	if err != nil {
		return false, err
	}

	return res == 1, nil
}
