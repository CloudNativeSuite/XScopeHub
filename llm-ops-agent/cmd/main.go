package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/yourname/XOpsAgent/api"
	"github.com/yourname/XOpsAgent/internal/config"
	"github.com/yourname/XOpsAgent/internal/server"
	"github.com/yourname/XOpsAgent/pkg/telemetry"
)

type Config struct {
	Inputs struct {
		DB struct {
			PGURL string `yaml:"pgurl"`
		} `yaml:"db"`
		OTEL struct {
			Endpoint string `yaml:"endpoint"`
		} `yaml:"otel"`
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

func runAgent(cfgPath string) error {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logConnections(cfg)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	listen := cfg.Outputs.API.Listen
	if listen == "" {
		listen = ":8080"
	}

	log.Printf("XOpsAgent daemon listening on %s", listen)
	return http.ListenAndServe(listen, mux)
}

func runAPI() error {
	ctx := context.Background()
	cfg := config.Load()
	shutdown, err := telemetry.Init(ctx, "aiops", cfg.OtlpEndpoint)
	if err != nil {
		return fmt.Errorf("failed to init telemetry: %w", err)
	}
	defer func() { _ = shutdown(ctx) }()

	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	return srv.Run(ctx)
}

func logConnections(cfg *Config) {
	logger := slog.Default()
	db, err := sql.Open("pgx", cfg.Inputs.DB.PGURL)
	if err != nil {
		logger.Error("pg db connection", "error", err)
	} else if err := db.Ping(); err != nil {
		logger.Warn("pg db ping failed", "error", err)
	} else {
		logger.Info("pg db connection ok")
	}
	_ = db.Close()
	checkHTTPEndpoint(logger, "otel endpoint", cfg.Inputs.OTEL.Endpoint)
	checkHTTPEndpoint(logger, "models.embedder.endpoint", cfg.Models.Embedder.Endpoint)
	checkHTTPEndpoint(logger, "models.generator.endpoint", cfg.Models.Generator.Endpoint)
}

func checkHTTPEndpoint(logger *slog.Logger, name, url string) {
	if url == "" {
		logger.Debug(name + " not configured")
		return
	}
	resp, err := http.Get(url)
	if err != nil {
		logger.Warn(name+" unreachable", "endpoint", url, "error", err)
		return
	}
	resp.Body.Close()
	logger.Info(name+" reachable", "endpoint", url, "status", resp.StatusCode)
}

var (
	mode    string
	cfgPath string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "llm-ops-agent",
		Short: "LLM Ops Agent service",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch mode {
			case "agent":
				return runAgent(cfgPath)
			case "api":
				return runAPI()
			default:
				return fmt.Errorf("unknown mode: %s", mode)
			}
		},
	}
	rootCmd.Flags().StringVar(&mode, "mode", "agent", "mode to run: agent or api")
	rootCmd.Flags().StringVar(&cfgPath, "config", "/etc/XOpsAgent.yaml", "path to config file (agent mode)")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
