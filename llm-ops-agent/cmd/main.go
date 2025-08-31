package main

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	daemon "github.com/sevlyar/go-daemon"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/yourname/XOpsAgent/api"
)

type Config struct {
	Server struct {
		API struct {
			Listen         string `yaml:"listen"`
			ResponseFormat string `yaml:"response_format"`
		} `yaml:"api"`
	} `yaml:"server"`
	Inputs struct {
		Postgres struct {
			URL string `yaml:"url"`
		} `yaml:"postgres"`
		OpenObserve struct {
			Endpoint string            `yaml:"endpoint"`
			Headers  map[string]string `yaml:"headers"`
		} `yaml:"openobserve"`
	} `yaml:"inputs"`
	Models struct {
		Embedder struct {
			Name     string `yaml:"name"`
			Endpoint string `yaml:"endpoint"`
		} `yaml:"embedder"`
		Generator struct {
			Models   []string `yaml:"models"`
			Endpoint string   `yaml:"endpoint"`
		} `yaml:"generator"`
	} `yaml:"models"`
	Outputs struct {
		GitHubPR struct {
			Enabled  bool   `yaml:"enabled"`
			Repo     string `yaml:"repo"`
			TokenEnv string `yaml:"token_env"`
			PR       struct {
				Number        int    `yaml:"number"`
				Title         string `yaml:"title"`
				Branch        string `yaml:"branch"`
				CommitMessage string `yaml:"commit_message"`
				Files         []struct {
					From string `yaml:"from"`
					To   string `yaml:"to"`
				} `yaml:"files"`
			} `yaml:"pr"`
		} `yaml:"github_pr"`
		FileReport struct {
			Enabled bool   `yaml:"enabled"`
			Path    string `yaml:"path"`
			Format  string `yaml:"format"`
		} `yaml:"file_report"`
		Answer struct {
			Enabled bool   `yaml:"enabled"`
			Channel string `yaml:"channel"`
		} `yaml:"answer"`
		Webhook struct {
			Enabled bool              `yaml:"enabled"`
			URL     string            `yaml:"url"`
			Headers map[string]string `yaml:"headers"`
		} `yaml:"webhook"`
	} `yaml:"outputs"`
	Routing struct {
		Default  []string            `yaml:"default"`
		OnAction map[string][]string `yaml:"on_action"`
		OnError  []string            `yaml:"on_error"`
	} `yaml:"routing"`
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

	listen := cfg.Server.API.Listen
	if listen == "" {
		return fmt.Errorf("server.api.listen must be set in config")
	}

	log.Printf("XOpsAgent daemon listening on %s", listen)
	return http.ListenAndServe(listen, mux)
}

func logConnections(cfg *Config) {
	logger := slog.Default()
	checkPostgres(logger, cfg.Inputs.Postgres.URL)
	checkHTTP(logger, "inputs.openobserve", cfg.Inputs.OpenObserve.Endpoint, cfg.Inputs.OpenObserve.Headers)
	checkHTTP(logger, "models.embedder", cfg.Models.Embedder.Endpoint, nil)
	checkHTTP(logger, "models.generator", cfg.Models.Generator.Endpoint, nil)

	if cfg.Outputs.GitHubPR.Enabled {
		checkGitHub(logger, cfg.Outputs.GitHubPR.Repo, cfg.Outputs.GitHubPR.TokenEnv)
	}
	if cfg.Outputs.FileReport.Enabled {
		checkFilePath(logger, cfg.Outputs.FileReport.Path)
	}
	if cfg.Outputs.Answer.Enabled {
		logger.Info("outputs.answer configured", "channel", cfg.Outputs.Answer.Channel)
	}
	if cfg.Outputs.Webhook.Enabled {
		checkHTTP(logger, "outputs.webhook", cfg.Outputs.Webhook.URL, cfg.Outputs.Webhook.Headers)
	}
}

func checkPostgres(logger *slog.Logger, url string) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		logger.Error("inputs.postgres connection", "error", err)
		return
	}
	if err := db.Ping(); err != nil {
		logger.Warn("inputs.postgres ping failed", "error", err)
	} else {
		logger.Info("inputs.postgres reachable")
	}
	_ = db.Close()
}

func checkHTTP(logger *slog.Logger, name, url string, headers map[string]string) {
	if url == "" {
		logger.Debug(name + " not configured")
		return
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error(name+" request", "endpoint", url, "error", err)
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	logger.Info("request", "target", name, "endpoint", url)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warn(name+" unreachable", "endpoint", url, "error", err)
		return
	}
	resp.Body.Close()
	logger.Info(name+" reachable", "endpoint", url, "status", resp.StatusCode)
}

func checkFilePath(logger *slog.Logger, path string) {
	if path == "" {
		logger.Debug("outputs.file_report not configured")
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("outputs.file_report inaccessible", "path", path, "error", err)
	} else {
		logger.Info("outputs.file_report path ok", "path", path)
	}
}

func checkGitHub(logger *slog.Logger, repo, tokenEnv string) {
	if repo == "" {
		logger.Debug("outputs.github_pr repo not configured")
		return
	}
	token := os.Getenv(tokenEnv)
	headers := map[string]string{}
	if token == "" {
		logger.Warn("outputs.github_pr token missing", "env", tokenEnv)
	} else {
		headers["Authorization"] = "Bearer " + token
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s", repo)
	checkHTTP(logger, "outputs.github_pr", url, headers)
}

var (
	daemonMode bool
	cfgPath    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "llm-ops-agent",
		Short: "LLM Ops Agent service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if daemonMode {
				cntxt := &daemon.Context{
					PidFileName: "xopsagent.pid",
					PidFilePerm: 0644,
				}
				child, err := cntxt.Reborn()
				if err != nil {
					return err
				}
				if child != nil {
					return nil
				}
				defer cntxt.Release()
			}
			return runAgent(cfgPath)
		},
	}
	rootCmd.PersistentFlags().BoolVar(&daemonMode, "daemon", true, "run in background")
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "/etc/XOpsAgent.yaml", "path to config file")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
