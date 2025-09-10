package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	daemon "github.com/sevlyar/go-daemon"
	"github.com/spf13/cobra"

	"github.com/xscopehub/xscopehub/internal/etl"
	"github.com/xscopehub/xscopehub/internal/etl/config"

	_ "github.com/lib/pq"
)

var (
	daemonMode bool
	configPath string
)

func checkPostgres(url string) error {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}

func checkEndpoint(url string) error {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func checkRepo(repoURL, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if token != "" {
		if u, err := url.Parse(repoURL); err == nil {
			u.User = url.UserPassword("token", token)
			repoURL = u.String()
		}
	}

	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd.Run()
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "xscopehub-etl",
		Short: "XScopeHub ETL service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if daemonMode {
				cntxt := &daemon.Context{
					PidFileName: "xscopehub-etl.pid",
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
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			log.Printf("INFO: loaded config from %s", configPath)

			log.Printf("DEBUG: checking Postgres connection %s", cfg.Outputs.Postgres.URL)
			if err := checkPostgres(cfg.Outputs.Postgres.URL); err != nil {
				log.Printf("ERROR: Postgres connection failed: %v", err)
			} else {
				log.Printf("INFO: Postgres connection successful")
			}

			log.Printf("DEBUG: checking OTEL endpoint %s", cfg.Inputs.OpenObserve.Endpoint)
			if err := checkEndpoint(cfg.Inputs.OpenObserve.Endpoint); err != nil {
				log.Printf("WARN: OTEL endpoint unreachable: %v", err)
			} else {
				log.Printf("INFO: OTEL endpoint reachable")
			}

			for _, repo := range cfg.Inputs.Ansible.Repos {
				log.Printf("DEBUG: checking ansible repo %s (%s)", repo.ID, repo.URL)
				token := ""
				if repo.Auth.TokenEnv != "" {
					token = os.Getenv(repo.Auth.TokenEnv)
				}
				if err := checkRepo(repo.URL, token); err != nil {
					log.Printf("WARN: ansible repo %s unreachable: %v", repo.ID, err)
				} else {
					log.Printf("INFO: ansible repo %s reachable", repo.ID)
				}
			}

			srv := etl.NewServer(cfg)
			return srv.Run()
		},
	}
	rootCmd.PersistentFlags().BoolVar(&daemonMode, "daemon", false, "run in background")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "config/observe-bridge-etl.yaml", "path to configuration file")
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
