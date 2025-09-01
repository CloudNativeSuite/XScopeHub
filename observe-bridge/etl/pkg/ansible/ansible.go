package ansible

import "context"

// Edge represents a dependency edge extracted from Ansible.
type Edge struct {
	From string
	To   string
}

// ExtractDeps extracts dependency edges for the tenant.
func ExtractDeps(ctx context.Context, tenant string) ([]Edge, error) {
	// TODO: implement Ansible dependency extraction
	return nil, nil
}
