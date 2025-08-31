package api

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// RegisterRoutes wires all HTTP handlers for the agent modules.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ingest/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "sensor", "status": "ok"})
	})
	mux.HandleFunc("/analyze/run", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "analyst", "status": "ok"})
	})
	mux.HandleFunc("/plan/generate", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "planner", "status": "ok"})
	})
	mux.HandleFunc("/gate/eval", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "gatekeeper", "status": "ok"})
	})
	mux.HandleFunc("/adapter/exec", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "executor", "status": "ok"})
	})
	mux.HandleFunc("/kb/ingest", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "librarian", "status": "ok"})
	})
	mux.HandleFunc("/case/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"module": "orchestrator", "status": "ok"})
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
}
