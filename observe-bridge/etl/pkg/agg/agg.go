package agg

// Record represents a processed record for aggregation.
type Record struct{}

// Feed ingests a record into the aggregator.
func Feed(rec Record) error {
	// TODO: implement aggregator feed
	return nil
}

// Drain returns aggregated results.
func Drain() ([]byte, error) {
	// TODO: implement aggregator drain
	return nil, nil
}
