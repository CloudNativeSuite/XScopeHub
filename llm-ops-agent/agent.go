package main

import (
	"fmt"

	"github.com/yourname/XOpsAgent/analyzer"
	"github.com/yourname/XOpsAgent/internal/input"
	"github.com/yourname/XOpsAgent/internal/model"
	"github.com/yourname/XOpsAgent/internal/output"
)

func main() {
	// Load configuration (placeholder)
	cfg := model.LoadConfig("config.yaml")

	// Step 1: Who is in trouble
	metrics := input.FetchMetrics(cfg.MetricsSource)
	suspectServices := analyzer.AnalyzeAbnormalServices(metrics)

	// Step 2: When
	timeWindow := analyzer.DetectTimeRange(metrics, suspectServices)

	for _, svc := range suspectServices {
		// Step 3: Which request
		traceLogs := input.FetchTraces(cfg.TraceSource, svc, timeWindow)
		errorTraces := analyzer.FindErrorTraces(traceLogs)

		// Step 4: Where
		for _, trace := range errorTraces {
			rootSpan := analyzer.LocateRootSpan(trace)
			pod, node := analyzer.ResolveLocation(rootSpan)

			// Step 5: What
			cause := analyzer.InferRootCause(trace, pod, node)
			suggestion := analyzer.SuggestAction(cause)

			// Output result
			output.GenerateReport(svc, trace.ID, timeWindow, rootSpan, cause, suggestion)
		}
	}

	fmt.Println("analysis complete")
}
