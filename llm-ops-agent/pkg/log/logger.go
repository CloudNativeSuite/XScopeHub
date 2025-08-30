package log

import (
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// New returns a slog.Logger that writes to OpenTelemetry.
func New(service string) *slog.Logger {
	handler := otelslog.NewHandler(service)
	return slog.New(handler)
}
