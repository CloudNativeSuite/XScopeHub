package backend

import "encoding/json"

// Result represents a backend response payload and metadata.
type Result struct {
	Payload json.RawMessage
	Backend string
	Cost    int64
}

// UnsupportedError indicates a query is unsupported by the backend.
type UnsupportedError struct {
	Status  int
	Message string
}

func (e *UnsupportedError) Error() string {
	if e == nil {
		return "unsupported"
	}
	return e.Message
}
