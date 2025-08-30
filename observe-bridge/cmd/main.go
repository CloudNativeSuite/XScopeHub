package main

import (
	"log"

	daemon "github.com/sevlyar/go-daemon"
	"github.com/spf13/cobra"

	"github.com/xscopehub/xscopehub/internal/etl"
	"github.com/xscopehub/xscopehub/internal/etl/config"
)

var (
	daemonMode bool
	configPath string
)

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
