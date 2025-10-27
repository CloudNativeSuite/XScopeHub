package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/xscopehub/mcp-server/internal/registry"
	"github.com/xscopehub/mcp-server/internal/server"
	"github.com/xscopehub/mcp-server/pkg/manifest"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "serve":
		serve(os.Args[2:])
	case "manifest":
		printManifest(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <serve|manifest> [flags]\n", filepath.Base(os.Args[0]))
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8000", "Address to listen on")
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest file")
	readTimeout := fs.Duration("read-timeout", 5*time.Second, "HTTP server read timeout")
	writeTimeout := fs.Duration("write-timeout", 10*time.Second, "HTTP server write timeout")
	_ = fs.Parse(args)

	mf, err := manifest.Load(*manifestPath)
	if err != nil {
		log.Fatalf("failed to load manifest: %v", err)
	}

	reg := registry.New()
	reg.RegisterResources(registry.StaticResources())
	reg.RegisterTools(registry.StaticTools())

	srv := server.New(server.Options{
		Manifest:     mf,
		Registry:     reg,
		ReadTimeout:  *readTimeout,
		WriteTimeout: *writeTimeout,
	})

	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv,
		ReadTimeout:  *readTimeout,
		WriteTimeout: *writeTimeout,
	}

	go func() {
		log.Printf("mcp server listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func printManifest(args []string) {
	fs := flag.NewFlagSet("manifest", flag.ExitOnError)
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest file")
	_ = fs.Parse(args)

	mf, err := manifest.Load(*manifestPath)
	if err != nil {
		log.Fatalf("failed to load manifest: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(mf); err != nil {
		log.Fatalf("failed to encode manifest: %v", err)
	}
}
