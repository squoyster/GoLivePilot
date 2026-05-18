package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/squoyster/golivepilot/internal/app"
	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/server"
)

var (
	version = "dev"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	configPath := flag.String("config", env("GOLIVEPILOT_CONFIG", "/config/golivepilot.yml"), "path to config file")
	listenOverride := flag.String("listen", "", "override listen address")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if *listenOverride != "" {
		cfg.App.Listen = *listenOverride
	}
	if cfg.App.Listen == "" {
		cfg.App.Listen = ":3000"
	}

	operatorPSK := env(cfg.Auth.PSKEnv, "")
	if cfg.Auth.Mode != "" && cfg.Auth.Mode != "none" && operatorPSK == "" {
		return fmt.Errorf("auth mode %q requires env var %q", cfg.Auth.Mode, cfg.Auth.PSKEnv)
	}

	runtime := app.NewRuntime(cfg)

	srvWrapper := server.NewServer(cfg, runtime, operatorPSK, version)

	srv := &http.Server{
		Addr:              cfg.App.Listen,
		Handler:           srvWrapper.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		log.Printf("GoLivePilot %s listening on %s", version, cfg.App.Listen)
		if cfg.TLS.Enabled {
			errCh <- srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
			return
		}
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Printf("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runtime.StopAll()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}

	log.Printf("shutdown complete")
	return nil
}

func env(key, fallback string) string {
	if key == "" {
		return fallback
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
