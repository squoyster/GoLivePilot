package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("config file %s not found; using defaults", path)
			return cfg, nil
		}
		return nil, err
	}

	if strings.TrimSpace(string(b)) == "" {
		return cfg, nil
	}

	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse YAML %s: %w", path, err)
	}

	ApplyDefaults(cfg)

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:    "GoLivePilot",
			Listen:  ":3000",
			DataDir: "/data",
			UIMode:  "simple",
		},
		Auth: AuthConfig{
			Mode:   "none",
			PSKEnv: "GOLIVEPILOT_OPERATOR_PSK",
			Cookie: CookieConfig{
				Name:     "golivepilot_session",
				Secure:   false,
				SameSite: "Lax",
				TTL:      "12h",
			},
		},
		FFmpeg: FFmpegConfig{
			Binary:   "/usr/bin/ffmpeg",
			LogLevel: "info",
		},
	}
}

func ApplyDefaults(cfg *Config) {
	if cfg.App.Name == "" {
		cfg.App.Name = "GoLivePilot"
	}
	if cfg.App.Listen == "" {
		cfg.App.Listen = ":3000"
	}
	if cfg.App.DataDir == "" {
		cfg.App.DataDir = "/data"
	}
	if cfg.App.UIMode == "" {
		cfg.App.UIMode = "simple"
	}
	if cfg.Auth.PSKEnv == "" {
		cfg.Auth.PSKEnv = "GOLIVEPILOT_OPERATOR_PSK"
	}
	if cfg.Auth.Cookie.Name == "" {
		cfg.Auth.Cookie.Name = "golivepilot_session"
	}
	if cfg.Auth.Cookie.SameSite == "" {
		cfg.Auth.Cookie.SameSite = "Lax"
	}
	if cfg.Auth.Cookie.TTL == "" {
		cfg.Auth.Cookie.TTL = "12h"
	}
	if cfg.FFmpeg.Binary == "" {
		cfg.FFmpeg.Binary = "/usr/bin/ffmpeg"
	}
	if cfg.FFmpeg.LogLevel == "" {
		cfg.FFmpeg.LogLevel = "info"
	}
}
