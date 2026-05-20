package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.App.Name != "GoLivePilot" {
		t.Errorf("expected App.Name GoLivePilot, got %q", cfg.App.Name)
	}
	if cfg.FFmpeg.Binary != "/usr/bin/ffmpeg" {
		t.Errorf("expected FFmpeg.Binary /usr/bin/ffmpeg, got %q", cfg.FFmpeg.Binary)
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	ApplyDefaults(cfg)
	if cfg.App.Name != "GoLivePilot" {
		t.Errorf("expected App.Name GoLivePilot, got %q", cfg.App.Name)
	}
	if cfg.Auth.Cookie.TTL != "12h" {
		t.Errorf("expected Auth.Cookie.TTL 12h, got %q", cfg.Auth.Cookie.TTL)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid empty config",
			cfg:  &Config{},
			wantErr: false,
		},
		{
			name: "duplicate ingest id",
			cfg: &Config{
				Ingests: []IngestConfig{
					{ID: "cam1"},
					{ID: "cam1"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate profile id",
			cfg: &Config{
				Profiles: []ProfileConfig{
					{ID: "720p"},
					{ID: "720p"},
				},
			},
			wantErr: true,
		},
		{
			name: "target references missing ingest",
			cfg: &Config{
				Targets: []TargetConfig{
					{ID: "fb", Enabled: true, IngestID: "missing", RTMPSURLEnv: "FB_URL"},
				},
			},
			wantErr: true,
		},
		{
			name: "target references missing profile",
			cfg: &Config{
				Targets: []TargetConfig{
					{ID: "fb", Enabled: true, ProfileID: "missing", RTMPSURLEnv: "FB_URL"},
				},
			},
			wantErr: true,
		},
		{
			name: "enabled target missing rtmps_url_env",
			cfg: &Config{
				Targets: []TargetConfig{
					{ID: "fb", Enabled: true},
				},
			},
			wantErr: true,
		},
		{
			name: "tls enabled but missing cert",
			cfg: &Config{
				TLS: TLSConfig{Enabled: true, KeyFile: "key.pem"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePipeline(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid pipeline",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "mediamtx", Kind: "service"},
						{ID: "source", Kind: "source.slate"},
					},
					States: []StateConfig{
						{ID: "standby", Nodes: []string{"mediamtx"}},
						{ID: "preview", Nodes: []string{"mediamtx", "source"}},
					},
					Transitions: []TransitionConfig{
						{ID: "to_preview", From: "standby", To: "preview"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing node id",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "", Kind: "service"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate node id",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
						{ID: "node1", Kind: "source"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing state id",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "", Nodes: []string{"node1"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate state id",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "state1", Nodes: []string{"node1"}},
						{ID: "state1", Nodes: []string{"node1"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "state references missing node",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "state1", Nodes: []string{"node1", "missing"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "transition references missing from state",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "state1", Nodes: []string{"node1"}},
					},
					Transitions: []TransitionConfig{
						{ID: "t1", From: "missing", To: "state1"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "transition references missing to state",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "state1", Nodes: []string{"node1"}},
					},
					Transitions: []TransitionConfig{
						{ID: "t1", From: "state1", To: "missing"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "transition from any is valid",
			cfg: &Config{
				Pipeline: PipelineConfig{
					Nodes: []NodeConfig{
						{ID: "node1", Kind: "service"},
					},
					States: []StateConfig{
						{ID: "standby", Nodes: []string{"node1"}},
					},
					Transitions: []TransitionConfig{
						{ID: "t1", From: "any", To: "standby"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Test loading non-existent file uses defaults
	cfg, err := LoadConfig("non-existent.yml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.App.Name != "GoLivePilot" {
		t.Errorf("expected default App.Name, got %q", cfg.App.Name)
	}

	// Test loading valid YAML
	tmpFile, err := os.CreateTemp("", "config*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `
app:
  name: CustomName
targets:
  - id: fb
    enabled: true
    rtmps_url_env: FB_URL
`
	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err = LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.App.Name != "CustomName" {
		t.Errorf("expected App.Name CustomName, got %q", cfg.App.Name)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].ID != "fb" {
		t.Errorf("expected 1 target with ID fb")
	}
}
