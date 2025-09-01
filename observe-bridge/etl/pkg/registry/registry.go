package registry

// Job represents a registered ETL job.
type Job struct {
	Name string
}

// Register adds a job to the registry.
func Register(job Job) error {
	// TODO: implement job registration
	return nil
}
