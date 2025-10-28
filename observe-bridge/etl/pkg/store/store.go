package store

import (
        "sync"

        "github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Job represents a queued flush job for a tenant and window.
type Job struct {
        Tenant  string
        Window  window.Window
        Payload []byte
}

var (
        mu       sync.Mutex
        queue    []Job
        enqueued = make(map[string]bool)
)

func key(job Job) string {
        return job.Tenant + job.Window.From.UTC().String() + job.Window.To.UTC().String()
}

// EnqueueOnce enqueues a job if it has not been enqueued before.
func EnqueueOnce(job Job) error {
        mu.Lock()
        defer mu.Unlock()
        k := key(job)
        if enqueued[k] {
                return nil
        }
        enqueued[k] = true
        queue = append(queue, job)
        return nil
}

// Dequeue returns the next job if available.
func Dequeue() (Job, bool) {
        mu.Lock()
        defer mu.Unlock()
        if len(queue) == 0 {
                return Job{}, false
        }
        job := queue[0]
        queue = queue[1:]
        return job, true
}

// MarkDone marks the job as completed.
func MarkDone(job Job) error {
        mu.Lock()
        defer mu.Unlock()
        delete(enqueued, key(job))
        return nil
}

