package audit

import (
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"
)

// Entry describes a single audit log record.
type Entry struct {
	Tenant   string        `json:"tenant"`
	User     string        `json:"user"`
	Lang     string        `json:"lang"`
	Query    string        `json:"query"`
	Cost     int64         `json:"cost"`
	Duration time.Duration `json:"duration"`
	Cached   bool          `json:"cached"`
	Backend  string        `json:"backend"`
	Error    string        `json:"error,omitempty"`
	Time     time.Time     `json:"time"`
}

// Logger emits audit entries in JSON format.
type Logger struct {
	enabled bool
	mu      sync.Mutex
	out     io.Writer
}

// New creates a new audit logger writing to the provided writer.
func New(enabled bool, out io.Writer) *Logger {
	if out == nil {
		out = log.Writer()
	}
	return &Logger{enabled: enabled, out: out}
}

// Log writes an audit entry if enabled.
func (l *Logger) Log(entry Entry) {
	if !l.enabled {
		return
	}
	entry.Time = entry.Time.UTC()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Write(append(data, '\n'))
}
