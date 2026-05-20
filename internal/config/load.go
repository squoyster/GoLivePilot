package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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

// LoadConfigDir loads all YAML files from a directory and merges them.
// Files are loaded in alphabetical order; later files override earlier ones.
func LoadConfigDir(dir string) (*Config, error) {
	cfg := DefaultConfig()

	if dir == "" {
		return cfg, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("config directory %s not found; using defaults", dir)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config dir %s: %w", dir, err)
	}

	var ymlFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			ymlFiles = append(ymlFiles, filepath.Join(dir, name))
		}
	}
	sort.Strings(ymlFiles)

	if len(ymlFiles) == 0 {
		log.Printf("no YAML files in %s; using defaults", dir)
		return cfg, nil
	}

	for _, path := range ymlFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		if strings.TrimSpace(string(b)) == "" {
			continue
		}

		var partial map[string]interface{}
		if err := yaml.Unmarshal(b, &partial); err != nil {
			return nil, fmt.Errorf("parse YAML %s: %w", path, err)
		}

		merged, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("re-serialize config: %w", err)
		}

		var mergedMap map[string]interface{}
		if err := yaml.Unmarshal(merged, &mergedMap); err != nil {
			return nil, fmt.Errorf("re-parse config: %w", err)
		}

		deepMerge(mergedMap, partial)

		remarshaled, err := yaml.Marshal(mergedMap)
		if err != nil {
			return nil, fmt.Errorf("re-serialize merged config: %w", err)
		}

		if err := yaml.Unmarshal(remarshaled, cfg); err != nil {
			return nil, fmt.Errorf("parse merged config: %w", err)
		}

		log.Printf("loaded config: %s", filepath.Base(path))
	}

	ApplyDefaults(cfg)

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func deepMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		if dstMap, ok := dst[k].(map[string]interface{}); ok {
			if srcMap, ok := v.(map[string]interface{}); ok {
				deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
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
	if cfg.Behavior.RestarterInterval == "" {
		cfg.Behavior.RestarterInterval = "5s"
	}
}
