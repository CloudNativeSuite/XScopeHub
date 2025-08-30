package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/xscopehub/xscopehub/internal/config"
)

func main() {
	cfgPath := flag.String("config", "configs/XOpsAgent.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadXOpsAgentConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("DB connection: %s", cfg.Inputs.DB.PgURL)
	log.Printf("OTEL endpoint: %s", cfg.Inputs.OTEL.Endpoint)
	log.Printf("Embedder model: %s at %s", cfg.Models.Embedder.Name, cfg.Models.Embedder.Endpoint)
	log.Printf("Generator models: %v", cfg.Models.Generator.Models)
	log.Printf("GitHub PR enabled: %v repo: %s", cfg.Outputs.GitHubPR.Enabled, cfg.Outputs.GitHubPR.Repo)
	log.Printf("File report path: %s format: %s", cfg.Outputs.FileReport.Path, cfg.Outputs.FileReport.Format)
	log.Printf("Answer channel: %s enabled: %v", cfg.Outputs.Answer.Channel, cfg.Outputs.Answer.Enabled)
	log.Printf("Webhook enabled: %v url: %s", cfg.Outputs.Webhook.Enabled, cfg.Outputs.Webhook.URL)
	log.Printf("Routing default sinks: %v", cfg.Routing.Default)
	log.Printf("Routing on_action: %v", cfg.Routing.OnAction)
	log.Printf("Routing on_error sinks: %v", cfg.Routing.OnError)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch cfg.Server.API.ResponseFormat {
		case "json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		case "ndjson":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Write([]byte(`{"status":"ok"}` + "\n"))
		default:
			w.Write([]byte("ok"))
		}
	})

	log.Printf("starting server on %s", cfg.Server.API.Listen)
	log.Fatal(http.ListenAndServe(cfg.Server.API.Listen, nil))
}
