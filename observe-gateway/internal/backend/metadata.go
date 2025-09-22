package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xscopehub/observe-gateway/internal/config"
)

var errTenantNotFound = errors.New("tenant metadata not found")

type tenantMetadata struct {
	Org        string
	LogTable   string
	TraceTable string
}

type metadataStore struct {
	pool        *pgxpool.Pool
	tenantQuery string
}

func newMetadataStore(ctx context.Context, cfg config.MetadataConfig) (*metadataStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, fmt.Errorf("metadata dsn required")
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse metadata dsn: %w", err)
	}
	if cfg.MaxConnections > 0 {
		poolConfig.MaxConns = cfg.MaxConnections
	}
	if cfg.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect metadata db: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping metadata db: %w", err)
	}

	query := strings.TrimSpace(cfg.TenantLookupQuery)
	if query == "" {
		query = "SELECT org, log_table, trace_table FROM tenant_metadata WHERE tenant = $1"
	}

	return &metadataStore{pool: pool, tenantQuery: query}, nil
}

func (s *metadataStore) Lookup(ctx context.Context, tenant string) (tenantMetadata, error) {
	if s == nil {
		return tenantMetadata{}, errTenantNotFound
	}

	row := s.pool.QueryRow(ctx, s.tenantQuery, tenant)

	var meta tenantMetadata
	if err := row.Scan(&meta.Org, &meta.LogTable, &meta.TraceTable); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenantMetadata{}, errTenantNotFound
		}
		return tenantMetadata{}, err
	}

	return meta, nil
}

func (s *metadataStore) Close() {
	if s == nil {
		return
	}
	s.pool.Close()
}
