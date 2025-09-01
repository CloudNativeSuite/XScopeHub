package store

// Job represents a queued job.
type Job struct{}

// EnqueueOnce enqueues a job if it has not been enqueued before.
func EnqueueOnce(job Job) error {
	// TODO: implement enqueue-once logic
	return nil
}

// MarkDone marks the job as completed.
func MarkDone(job Job) error {
	// TODO: implement job completion mark
	return nil
}
