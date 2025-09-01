package iac

import "context"

// Edge represents a discovered topology edge.
type Edge struct {
	From string
	To   string
}

// Discover extracts infrastructure edges for the tenant.
func Discover(ctx context.Context, tenant string) ([]Edge, error) {
	// TODO: implement IaC discovery
	return nil, nil
}
