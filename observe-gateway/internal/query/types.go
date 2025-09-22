package query

import (
	"encoding/json"
	"time"
)

// Request defines the payload for POST /api/query.
type Request struct {
	Lang      string    `json:"lang"`
	Query     string    `json:"query"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Step      string    `json:"step"`
	Normalize bool      `json:"normalize"`
}

// Response wraps upstream responses with additional metadata.
type Response struct {
	Lang   string          `json:"lang"`
	Tenant string          `json:"tenant"`
	Result json.RawMessage `json:"result"`
	Stats  Stats           `json:"stats"`
}

// Stats describes runtime statistics.
type Stats struct {
	Backend    string `json:"backend"`
	Cached     bool   `json:"cached"`
	DurationMS int64  `json:"duration_ms"`
	Cost       int64  `json:"cost"`
}

// HasTimeRange returns true when the request is a range query.
func (r Request) HasTimeRange() bool {
	return !r.Start.IsZero() && !r.End.IsZero()
}

// StepDuration parses the step duration if provided.
func (r Request) StepDuration() (time.Duration, error) {
	if r.Step == "" {
		return 0, nil
	}
	return time.ParseDuration(r.Step)
}
