package analyzer

import (
	"github.com/yourname/XOpsAgent/internal/model"
)

// InferRootCause tries to infer the root cause of a trace.
func InferRootCause(trace model.Trace, pod, node string) model.RootCause {
	if last := lastNetworkSpan(trace); last != nil {
		if val, ok := last.Tags["tcp_reset"]; ok && val == "true" {
			return model.RootCause{
				Type:        "TCP Reset",
				Description: "Server closed connection via TCP RST",
				Evidence:    []string{"network_span:tcp_reset=true"},
			}
		}
	}

	if podHasOOM(pod) {
		return model.RootCause{
			Type:        "OOM",
			Description: "Pod restarted due to out-of-memory",
			Evidence:    []string{"pod_event:OOMKilled"},
		}
	}

	return model.RootCause{
		Type:        "Unknown",
		Description: "Root cause not found",
	}
}

// Helpers below are minimal stubs to make code compile.
func lastNetworkSpan(trace model.Trace) *model.Span { return nil }
func podHasOOM(pod string) bool                     { return false }
