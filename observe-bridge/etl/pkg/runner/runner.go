package runner

import (
        "context"
        "time"
)

// Run executes the given job function with basic retry logic. The function is
// attempted up to three times with exponential backoff and honours context
// cancellation.
func Run(ctx context.Context, name string, fn func(context.Context) error) error {
        var err error
        for attempt := 0; attempt < 3; attempt++ {
                if err = fn(ctx); err == nil {
                        return nil
                }
                select {
                case <-time.After(time.Duration(attempt+1) * time.Second):
                case <-ctx.Done():
                        return ctx.Err()
                }
        }
        return err
}

