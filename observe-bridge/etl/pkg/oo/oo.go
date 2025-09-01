package oo

import (
	"context"

	"github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Record represents a generic OpenObserve record.
type Record struct{}

// Stream reads records for the tenant in the given window and invokes fn for each record.
func Stream(ctx context.Context, tenant string, w window.Window, fn func(Record)) error {
	// TODO: implement streaming from OpenObserve
	return nil
}
