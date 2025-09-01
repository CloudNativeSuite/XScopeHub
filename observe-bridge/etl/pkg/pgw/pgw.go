package pgw

import (
	"context"

	"github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Flush writes aggregated output to Postgres.
func Flush(ctx context.Context, tenant string, w window.Window, out []byte) error {
	// TODO: implement Postgres flush
	return nil
}

// Edge represents a topology edge.
type Edge struct {
	From string
	To   string
}

// UpsertTopoEdges upserts topology edges for the tenant.
func UpsertTopoEdges(ctx context.Context, tenant string, edges []Edge) error {
	// TODO: implement topology edge upsert
	return nil
}
