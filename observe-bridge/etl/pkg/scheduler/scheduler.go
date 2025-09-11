package scheduler

import (
        "context"

        "github.com/xscopehub/xscopehub/etl/pkg/agg"
        "github.com/xscopehub/xscopehub/etl/pkg/pgw"
        "github.com/xscopehub/xscopehub/etl/pkg/runner"
        "github.com/xscopehub/xscopehub/etl/pkg/store"
)

// Tick processes pending windows, enqueues flush jobs and executes them with
// retry semantics. It ensures each batch is written once to Postgres.
func Tick(ctx context.Context) error {
        batches := agg.Drain()
        for tenant, wmap := range batches {
                for w, payload := range wmap {
                        job := store.Job{Tenant: tenant, Window: w, Payload: payload}
                        if err := store.EnqueueOnce(job); err != nil {
                                return err
                        }
                }
        }

        for {
                job, ok := store.Dequeue()
                if !ok {
                        break
                }
                if err := runner.Run(ctx, "pgw.Flush", func(ctx context.Context) error {
                        if err := pgw.Flush(ctx, job.Tenant, job.Window, job.Payload); err != nil {
                                return err
                        }
                        return store.MarkDone(job)
                }); err != nil {
                        return err
                }
        }
        return nil
}

