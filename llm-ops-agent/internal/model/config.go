package model

// Config stores data source addresses and rule parameters.
type Config struct {
	MetricsSource string
	TraceSource   string
}

// LoadConfig loads configuration from a file path.
func LoadConfig(path string) Config {
	// Placeholder implementation
	return Config{}
}
