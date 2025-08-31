package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the ETL service configuration.
type Config struct {
	Server struct {
		API struct {
			Listen         string `yaml:"listen"`
			ResponseFormat string `yaml:"response_format"`
		} `yaml:"api"`
	} `yaml:"server"`
	Inputs struct {
		OpenObserve struct {
			Endpoint string            `yaml:"endpoint"`
			Headers  map[string]string `yaml:"headers"`
			Datasets map[string]string `yaml:"datasets"`
		} `yaml:"openobserve"`
		Ansible struct {
			Repos []struct {
				ID   string `yaml:"id"`
				URL  string `yaml:"url"`
				Ref  string `yaml:"ref"`
				Auth struct {
					TokenEnv string `yaml:"token_env"`
				} `yaml:"auth"`
				Inventory []string `yaml:"inventory"`
				Playbooks []string `yaml:"playbooks"`
				Vars      []string `yaml:"vars"`
			} `yaml:"repos"`
		} `yaml:"ansible"`
	} `yaml:"inputs"`
	Outputs struct {
		Postgres struct {
			URL string `yaml:"url"`
		} `yaml:"postgres"`
	} `yaml:"outputs"`
	Scheduler struct {
		Jitter      string `yaml:"jitter"`
		MaxBackfill string `yaml:"max_backfill"`
		Reload      struct {
			FSWatch bool `yaml:"fs_watch"`
		} `yaml:"reload"`
	} `yaml:"scheduler"`
	Tenants struct {
		InitialLookback map[string]string `yaml:"initial_lookback"`
		List            []struct {
			Code string `yaml:"code"`
			ID   int    `yaml:"id"`
		} `yaml:"list"`
	} `yaml:"tenants"`
	Jobs map[string]struct {
		Enabled     bool     `yaml:"enabled"`
		Align       string   `yaml:"align,omitempty"`
		Delay       string   `yaml:"delay,omitempty"`
		Interval    string   `yaml:"interval,omitempty"`
		Concurrency int      `yaml:"concurrency,omitempty"`
		DependsOn   []string `yaml:"depends_on,omitempty"`
		Graph       struct {
			Name    string `yaml:"name"`
			SQLFile string `yaml:"sql_file"`
		} `yaml:"graph,omitempty"`
		StatusRef      string `yaml:"status_ref,omitempty"`
		FullSyncOnBoot bool   `yaml:"full_sync_on_boot,omitempty"`
		DriftDetection struct {
			Enabled          bool   `yaml:"enabled"`
			EmitEvent        bool   `yaml:"emit_event"`
			SeverityOnChange string `yaml:"severity_on_change"`
		} `yaml:"drift_detection,omitempty"`
		RepoRef      string `yaml:"repo_ref,omitempty"`
		ChangeWindow string `yaml:"change_window,omitempty"`
	} `yaml:"jobs"`
}

// Load reads configuration from the given file path and decodes it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expanded := os.ExpandEnv(string(data))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
