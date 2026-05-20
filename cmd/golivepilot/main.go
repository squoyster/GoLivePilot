package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/squoyster/golivepilot/internal/app"
	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/server"
)

var (
	version = "0.1.46"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GoLivePilot - Live Stream Management via MediaMTX and FFmpeg\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  golivepilot [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nFor more information on environment variables, use --env-help\n")
	}

	configPath := flag.String("config", env("GOLIVEPILOT_CONFIG", "/config/golivepilot.yml"), "path to config file")
	listenOverride := flag.String("listen", "", "override listen address")
	envHelp := flag.Bool("env-help", false, "show relevant environment variables and exit")
	flag.Parse()

	if *envHelp {
		showEnvHelp()
		return nil
	}

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

	setupLogging(cfg.Logging)

	operatorPSK := env(cfg.Auth.PSKEnv, "")
	if cfg.Auth.Mode != "" && cfg.Auth.Mode != "none" && operatorPSK == "" {
		return fmt.Errorf("auth mode %q requires env var %q", cfg.Auth.Mode, cfg.Auth.PSKEnv)
	}

	supervisor := ffmpeg.NewSupervisor()
	runtime := app.NewRuntime(cfg, supervisor)

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
		slog.Info("server starting", "version", version, "listen", cfg.App.Listen)
		if cfg.TLS.Enabled {
			errCh <- srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
			return
		}
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown requested")
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
	slog.Info("shutdown complete")
	return nil
}

func setupLogging(cfg config.LoggingConfig) {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source, ok := a.Value.Any().(*slog.Source)
				if ok {
					// Ensure we use forward slashes for cross-platform consistency in logs
					path := filepath.ToSlash(source.File)
					parts := strings.Split(path, "/")
					if len(parts) >= 2 {
						source.File = parts[len(parts)-2] + "/" + parts[len(parts)-1]
					} else {
						source.File = path
					}
				}
			}
			return a
		},
	}
	var handler slog.Handler

	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
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

func showEnvHelp() {
	fmt.Println("Relevant Environment Variables:")
	fmt.Println(strings.Repeat("-", 40))
	for _, ev := range config.RelevantEnvVars {
		fmt.Printf("Name:        %s\n", ev.Name)
		fmt.Printf("Description: %s\n", ev.Description)
		fmt.Printf("Example:     %s\n", ev.Example)
		fmt.Println(strings.Repeat("-", 40))
	}
}
