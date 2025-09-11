package pgw

import (
        "context"
        "fmt"
        "os"
        "sync"

        "github.com/jackc/pgx/v5"
        "github.com/jackc/pgx/v5/pgxpool"

        "github.com/xscopehub/xscopehub/etl/pkg/window"
)

var (
        pool *pgxpool.Pool
        once sync.Once
)

// getPool lazily initialises the Postgres connection pool.
func getPool(ctx context.Context) (*pgxpool.Pool, error) {
        var err error
        once.Do(func() {
                dsn := os.Getenv("OUTPUT_PG_DSN")
                if dsn == "" {
                        err = fmt.Errorf("OUTPUT_PG_DSN not set")
                        return
                }
                pool, err = pgxpool.New(ctx, dsn)
        })
        return pool, err
}

// Flush writes aggregated output to Postgres. Each call performs an UPSERT
// into the `agg_output` table to ensure results are persisted once per window
// and tenant.
func Flush(ctx context.Context, tenant string, w window.Window, out []byte) error {
        p, err := getPool(ctx)
        if err != nil {
                return err
        }
        _, err = p.Exec(ctx,
                `INSERT INTO agg_output (tenant, window_start, window_end, payload)
                 VALUES ($1,$2,$3,$4)
                 ON CONFLICT (tenant, window_start, window_end) DO UPDATE SET payload = EXCLUDED.payload`,
                tenant, w.From.UTC(), w.To.UTC(), out)
        return err
}

// Edge represents a topology edge.
type Edge struct {
        From string
        To   string
}

// UpsertTopoEdges upserts topology edges for the tenant. Edges are written with
// idempotent semantics using INSERT ... ON CONFLICT DO NOTHING.
func UpsertTopoEdges(ctx context.Context, tenant string, edges []Edge) error {
        if len(edges) == 0 {
                return nil
        }
        p, err := getPool(ctx)
        if err != nil {
                return err
        }
        batch := &pgx.Batch{}
        for _, e := range edges {
                batch.Queue(`INSERT INTO topo_edges (tenant, from_node, to_node)
                             VALUES ($1,$2,$3)
                             ON CONFLICT (tenant, from_node, to_node) DO NOTHING`,
                        tenant, e.From, e.To)
        }
        br := p.SendBatch(ctx, batch)
        return br.Close()
}

