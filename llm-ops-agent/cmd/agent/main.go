package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/yourname/XOpsAgent/api"
	"github.com/yourname/XOpsAgent/internal/config"
	"github.com/yourname/XOpsAgent/internal/server"
	"github.com/yourname/XOpsAgent/pkg/telemetry"
)

type Config struct {
	Inputs struct {
		DB struct {
			PGURL        string   `yaml:"pgurl"`
			OTelEndpoint []string `yaml:"otel_endpoint"`
		} `yaml:"db"`
	} `yaml:"inputs"`
	Outputs struct {
		API struct {
			Listen string `yaml:"listen"`
			Type   string `yaml:"type"`
		} `yaml:"api"`
		GitOps []struct {
			RepoURL string `yaml:"repoUrl"`
			Token   string `yaml:"token"`
		} `yaml:"gitops"`
	} `yaml:"outputs"`
	Models struct {
		Embedder struct {
			Models   string `yaml:"models"`
			Endpoint string `yaml:"endpoint"`
		} `yaml:"embedder"`
		Generator struct {
			Models   []string `yaml:"models"`
			Endpoint string   `yaml:"endpoint"`
		} `yaml:"generator"`
	} `yaml:"models"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func runAgent(cfgPath string) {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	listen := cfg.Outputs.API.Listen
	if listen == "" {
		listen = ":8080"
	}

	log.Printf("XOpsAgent daemon listening on %s", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func runAPI() {
	ctx := context.Background()
	cfg := config.Load()
	shutdown, err := telemetry.Init(ctx, "aiops", cfg.OtlpEndpoint)
	if err != nil {
		log.Fatalf("failed to init telemetry: %v", err)
	}
	defer func() { _ = shutdown(ctx) }()

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server run: %v", err)
	}
}

func main() {
	mode := flag.String("mode", "agent", "mode to run: agent or api")
	cfgPath := flag.String("config", "/etc/XOpsAgent.yaml", "path to config file (agent mode)")
	flag.Parse()

	switch *mode {
	case "agent":
		runAgent(*cfgPath)
	case "api":
		runAPI()
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}
