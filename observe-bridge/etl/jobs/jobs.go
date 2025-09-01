package jobs

import (
	"context"

	"github.com/xscopehub/xscopehub/etl/pkg/window"
)

// RunOOAgg aggregates OpenObserve data into Postgres.
func RunOOAgg(ctx context.Context, tenant string, w window.Window) error {
	// TODO: implement OO aggregation job
	return nil
}

// RunAGERefresh refreshes the active graph edges.
func RunAGERefresh(ctx context.Context, tenant string, w window.Window) error {
	// TODO: implement AGE refresh job
	return nil
}

// RunTopoIAC processes IaC topology edges.
func RunTopoIAC(ctx context.Context, tenant string, w window.Window) error {
	// TODO: implement topology IaC job
	return nil
}

// RunTopoAnsible processes Ansible topology edges.
func RunTopoAnsible(ctx context.Context, tenant string, w window.Window) error {
	// TODO: implement topology Ansible job
	return nil
}
