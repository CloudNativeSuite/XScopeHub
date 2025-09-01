package window

import "time"

// Window represents a time range for ETL operations.
type Window struct {
	From time.Time
	To   time.Time
}
