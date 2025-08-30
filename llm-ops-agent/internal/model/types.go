package model

import "time"

// Metric represents a metrics datapoint.
type Metric struct {
	ServiceName string
	Timestamp   time.Time
	LatencyAvg  float64
	LatencyMax  float64
	ErrorRate   float64
}

// Trace represents a distributed trace.
type Trace struct {
	ID    string
	Spans []Span
}

// Span represents an individual span in a trace.
type Span struct {
	ID         string
	ParentID   string
	Service    string
	Operation  string
	DurationMs int64
	Error      bool
	Tags       map[string]string
}

// RootCause describes the diagnosed root cause of an issue.
type RootCause struct {
	Type        string
	Description string
	Evidence    []string
}

// ActionSuggestion holds suggestions to remediate an issue.
type ActionSuggestion struct {
	Suggestions []string
}
