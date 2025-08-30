package config

import (
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

// XOpsAgentConfig defines configuration for XOps Agent.
type XOpsAgentConfig struct {
	Server struct {
		API struct {
			Listen         string `yaml:"listen"`
			ResponseFormat string `yaml:"response_format"`
		} `yaml:"api"`
	} `yaml:"server"`

	Inputs struct {
		DB struct {
			PgURL string `yaml:"pgurl"`
		} `yaml:"db"`
		OTEL struct {
			Endpoint string            `yaml:"endpoint"`
			Headers  map[string]string `yaml:"headers"`
		} `yaml:"otel"`
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

// LoadXOpsAgentConfig reads YAML configuration from the given path.
func LoadXOpsAgentConfig(path string) (*XOpsAgentConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg XOpsAgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
