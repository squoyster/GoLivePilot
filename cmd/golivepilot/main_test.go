package main

import (
	"os"
	"testing"

	"github.com/squoyster/golivepilot/internal/config"
)

func TestEnv(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		setEnv    map[string]string
		fallback  string
		expect    string
	}{
		{
			name:     "returns env value when set",
			key:      "TEST_ENV_VAR",
			setEnv:   map[string]string{"TEST_ENV_VAR": "actual-value"},
			fallback: "fallback",
			expect:   "actual-value",
		},
		{
			name:     "returns fallback when env not set",
			key:      "UNSET_ENV_VAR_12345",
			setEnv:   map[string]string{},
			fallback: "fallback-value",
			expect:   "fallback-value",
		},
		{
			name:     "returns fallback when key is empty",
			key:      "",
			setEnv:   map[string]string{},
			fallback: "fallback-value",
			expect:   "fallback-value",
		},
		{
			name:     "returns fallback when env is empty",
			key:      "EMPTY_ENV_VAR",
			setEnv:   map[string]string{"EMPTY_ENV_VAR": ""},
			fallback: "fallback",
			expect:   "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.setEnv {
				t.Setenv(k, v)
			}
			result := env(tt.key, tt.fallback)
			if result != tt.expect {
				t.Errorf("env(%q, %q) = %q, want %q", tt.key, tt.fallback, result, tt.expect)
			}
		})
	}
}

func TestShowEnvHelp(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showEnvHelp()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "" {
		t.Fatal("expected non-empty output from showEnvHelp")
	}
	if !contains(output, "Relevant Environment Variables:") {
		t.Error("expected header in output")
	}
	if !contains(output, "GOLIVEPILOT_CONFIG") {
		t.Error("expected GOLIVEPILOT_CONFIG in output")
	}
	if !contains(output, "GOLIVEPILOT_OPERATOR_PSK") {
		t.Error("expected GOLIVEPILOT_OPERATOR_PSK in output")
	}
}

func TestSetupLogging(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		format string
	}{
		{"text info", "info", "text"},
		{"text debug", "debug", "text"},
		{"text warn", "warn", "text"},
		{"text error", "error", "text"},
		{"json info", "info", "json"},
		{"json debug", "debug", "json"},
		{"default level", "", "text"},
		{"default format", "info", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.LoggingConfig{Level: tt.level, Format: tt.format}
			setupLogging(cfg)
		})
	}
}

// runWithArgs runs the main logic with the given arguments.
func runWithArgs(args []string) error {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = args
	return run()
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRun_InvalidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yml"
	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte(":\tinvalid: yaml: ["), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := runWithArgs([]string{"golivepilot", "-config", configPath})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !contains(err.Error(), "load config") {
		t.Errorf("expected 'load config' in error, got %v", err)
	}
}
