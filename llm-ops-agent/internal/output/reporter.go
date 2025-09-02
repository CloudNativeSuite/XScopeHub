package output

import (
	"fmt"

	"github.com/yourname/XOpsAgent/internal/model"
	"github.com/yourname/XOpsAgent/internal/utils"
)

// GenerateReport prints an analysis report.
func GenerateReport(service, traceID string, timeRange utils.TimeRange, rootSpan model.Span, cause model.RootCause, suggestion model.ActionSuggestion) {
	fmt.Printf("Service: %s\n", service)
	fmt.Printf("TraceID: %s\n", traceID)
	fmt.Printf("Time Range: %s - %s\n", timeRange.Start, timeRange.End)
	fmt.Printf("Root Span: %s\n", rootSpan.Operation)
	fmt.Printf("Root Cause: %s\nDetails: %s\n", cause.Type, cause.Description)
	fmt.Println("Evidence:")
	for _, e := range cause.Evidence {
		fmt.Printf("  - %s\n", e)
	}
	fmt.Println("Action Suggestions:")
	for _, s := range suggestion.Suggestions {
		fmt.Printf("  - %s\n", s)
	}
}
