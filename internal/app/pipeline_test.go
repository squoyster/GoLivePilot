package app

import (
	"context"
	"testing"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/mediamtx"
)

func pipelineConfig() *config.Config {
	cfg := testConfig()
	cfg.Pipeline.Nodes = []config.NodeConfig{
		{ID: "mediamtx", Label: "MediaMTX", Kind: "service"},
		{
			ID:        "__internal_source__",
			Label:     "Slate Source",
			Kind:      "source.slate",
			DependsOn: []string{"mediamtx"},
			Input:     "assets/starting-soon.png",
			Output:    "rtmp://localhost:1935/live/internal-program",
			Timeout:   "5s",
		},
		{
			ID:        "camera_source",
			Label:     "Camera Source",
			Kind:      "source.rtmp",
			DependsOn: []string{"mediamtx"},
			Input:     "rtmp://localhost:1935/live/camera",
			Output:    "rtmp://localhost:1935/live/internal-program",
			Timeout:   "5s",
		},
		{
			ID:        "__program_source__",
			Label:     "Program Output",
			Kind:      "stream.program",
			DependsOn: []string{"mediamtx"},
			Input:     "rtmp://localhost:1935/live/internal-program",
			Output:    "rtmp://localhost:1935/live/program",
			Timeout:   "5s",
		},
		{
			ID:        "__preview__",
			Label:     "Local Preview",
			Kind:      "relay.rtmp",
			DependsOn: []string{"__program_source__"},
			Input:     "rtmp://localhost:1935/live/program",
			Output:    "rtmp://localhost:1935/live/preview",
		},
		{
			ID:         "facebook-main",
			Label:      "Facebook",
			Kind:       "relay.rtmps",
			DependsOn:  []string{"__program_source__"},
			Input:      "rtmp://localhost:1935/live/program",
			OutputEnv:  "FACEBOOK_RTMPS_URL",
			OutputKeyEnv: "FACEBOOK_RTMPS_KEY",
		},
	}
	cfg.Pipeline.States = []config.StateConfig{
		{ID: "standby", Nodes: []string{"mediamtx"}},
		{ID: "preview", Nodes: []string{"mediamtx", "__internal_source__", "__program_source__", "__preview__", "facebook-main"}},
		{ID: "live", Nodes: []string{"mediamtx", "camera_source", "__program_source__", "__preview__", "facebook-main"}},
		{ID: "ended", Nodes: []string{"mediamtx", "__program_source__", "__preview__", "facebook-main"}},
	}
	cfg.Pipeline.Transitions = []config.TransitionConfig{
		{ID: "standby_to_preview", From: "standby", To: "preview"},
		{ID: "preview_to_live", From: "preview", To: "live"},
		{ID: "preview_to_ended", From: "preview", To: "ended"},
		{ID: "live_to_ended", From: "live", To: "ended"},
	}
	return cfg
}

func TestPipeline_NewPipeline(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}

	p := NewPipeline(cfg, sup, mtx)
	if p == nil {
		t.Fatal("expected pipeline")
	}
	if p.currentState != "standby" {
		t.Errorf("expected initial state standby, got %s", p.currentState)
	}
}

func TestPipeline_CurrentState(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}
	p := NewPipeline(cfg, sup, mtx)

	if p.CurrentState() != "standby" {
		t.Errorf("expected standby, got %s", p.CurrentState())
	}
}

func TestPipeline_Transition(t *testing.T) {
	t.Run("standby to preview", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
			waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
				return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
			},
		}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		err := p.Transition(ctx, "preview")
		if err != nil {
			t.Fatalf("transition failed: %v", err)
		}
		if p.CurrentState() != "preview" {
			t.Errorf("expected state preview, got %s", p.CurrentState())
		}
	})

	t.Run("preview to live", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
			waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
				return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
			},
		}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		_ = p.Transition(ctx, "preview")
		err := p.Transition(ctx, "live")
		if err != nil {
			t.Fatalf("transition to live failed: %v", err)
		}
		if p.CurrentState() != "live" {
			t.Errorf("expected state live, got %s", p.CurrentState())
		}
	})

	t.Run("live to ended", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
			waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
				return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
			},
		}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		_ = p.Transition(ctx, "preview")
		_ = p.Transition(ctx, "live")
		err := p.Transition(ctx, "ended")
		if err != nil {
			t.Fatalf("transition to ended failed: %v", err)
		}
		if p.CurrentState() != "ended" {
			t.Errorf("expected state ended, got %s", p.CurrentState())
		}
	})

	t.Run("any to standby", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
			waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
				return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
			},
		}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		_ = p.Transition(ctx, "preview")
		err := p.Transition(ctx, "standby")
		if err != nil {
			t.Fatalf("transition to standby failed: %v", err)
		}
		if p.CurrentState() != "standby" {
			t.Errorf("expected state standby, got %s", p.CurrentState())
		}
	})

	t.Run("invalid transition returns error", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		err := p.Transition(ctx, "live")
		if err == nil {
			t.Fatal("expected error for invalid transition from standby to live")
		}
	})
}

func TestPipeline_Transition_PersistentRelay(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{
		status: map[string]ffmpeg.RelayStatus{
			"facebook-main": {State: ffmpeg.StateRunning},
		},
	}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
			return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
		},
	}
	p := NewPipeline(cfg, sup, mtx)

	ctx := context.Background()
	_ = p.Transition(ctx, "preview")

	// Transition preview -> live should NOT stop persistent relay
	_ = p.Transition(ctx, "live")

	for _, id := range sup.stops {
		if id == "facebook-main" {
			t.Error("persistent facebook relay should not be stopped during preview->live")
		}
	}
}

func TestPipeline_FindNode(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}
	p := NewPipeline(cfg, sup, mtx)

	t.Run("finds existing node", func(t *testing.T) {
		node, ok := p.findNode("mediamtx")
		if !ok {
			t.Fatal("expected to find mediamtx node")
		}
		if node.Kind != "service" {
			t.Errorf("expected kind service, got %s", node.Kind)
		}
	})

	t.Run("returns false for missing node", func(t *testing.T) {
		_, ok := p.findNode("nonexistent")
		if ok {
			t.Error("expected false for nonexistent node")
		}
	})
}

func TestPipeline_EnsureNode(t *testing.T) {
	t.Run("service node checks health", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		started := make(map[string]bool)
		err := p.ensureNode(ctx, "mediamtx", started)
		if err != nil {
			t.Fatalf("ensureNode for mediamtx failed: %v", err)
		}
		if !started["mediamtx"] {
			t.Error("expected mediamtx to be marked as started")
		}
	})

	t.Run("returns error for missing node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		started := make(map[string]bool)
		err := p.ensureNode(ctx, "nonexistent", started)
		if err == nil {
			t.Fatal("expected error for nonexistent node")
		}
	})

	t.Run("skips already running node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{
			status: map[string]ffmpeg.RelayStatus{
				"__internal_source__": {State: ffmpeg.StateRunning},
			},
		}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		started := make(map[string]bool)
		err := p.ensureNode(ctx, "__internal_source__", started)
		if err != nil {
			t.Fatalf("ensureNode for running node failed: %v", err)
		}
		if !started["__internal_source__"] {
			t.Error("expected node to be marked as started")
		}
	})

	t.Run("unknown node kind returns error", func(t *testing.T) {
		cfg := pipelineConfig()
		cfg.Pipeline.Nodes = append(cfg.Pipeline.Nodes, config.NodeConfig{
			ID:    "unknown",
			Kind:  "unknown.kind",
		})
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		ctx := context.Background()
		started := make(map[string]bool)
		err := p.ensureNode(ctx, "unknown", started)
		if err == nil {
			t.Fatal("expected error for unknown node kind")
		}
	})
}

func TestPipeline_BuildStartRequest(t *testing.T) {
	t.Run("source.slate node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("__internal_source__")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Mode != "source" {
			t.Errorf("expected mode source, got %s", req.Mode)
		}
		if req.TargetID != "__internal_source__" {
			t.Errorf("expected target_id __internal_source__, got %s", req.TargetID)
		}
	})

	t.Run("source.rtmp node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("camera_source")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Mode != "source" {
			t.Errorf("expected mode source, got %s", req.Mode)
		}
	})

	t.Run("stream.program node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("__program_source__")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Mode != "program" {
			t.Errorf("expected mode program, got %s", req.Mode)
		}
	})

	t.Run("relay.rtmp node", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("__preview__")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Mode != "relay" {
			t.Errorf("expected mode relay, got %s", req.Mode)
		}
	})

	t.Run("relay.rtmps node with env vars", func(t *testing.T) {
		t.Setenv("FACEBOOK_RTMPS_URL", "rtmps://live-api-s.facebook.com:443/rtmp/")
		t.Setenv("FACEBOOK_RTMPS_KEY", "fb-key-123")

		cfg := pipelineConfig()
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("facebook-main")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Mode != "relay" {
			t.Errorf("expected mode relay, got %s", req.Mode)
		}
		if req.Output != "rtmps://live-api-s.facebook.com:443/rtmp/fb-key-123" {
			t.Errorf("expected full URL with key, got %s", req.Output)
		}
	})

	t.Run("relay.rtmps node literal URL fallback", func(t *testing.T) {
		cfg := pipelineConfig()
		// Replace the facebook-main node with one that has literal URLs in output_env
		for i, n := range cfg.Pipeline.Nodes {
			if n.ID == "facebook-main" {
				cfg.Pipeline.Nodes[i].OutputEnv = "rtmps://live-api-s.facebook.com:443/rtmp/"
				cfg.Pipeline.Nodes[i].OutputKeyEnv = "my-literal-key"
				break
			}
		}
		sup := &mockSupervisor{}
		mtx := &mockMediaMTXClient{}
		p := NewPipeline(cfg, sup, mtx)

		node, _ := p.findNode("facebook-main")
		req, err := p.buildStartRequest(node)
		if err != nil {
			t.Fatalf("buildStartRequest failed: %v", err)
		}
		if req.Output != "rtmps://live-api-s.facebook.com:443/rtmp/my-literal-key" {
			t.Errorf("expected full URL with key from literal fallback, got %s", req.Output)
		}
	})
}

func TestPipeline_GetTranscodeArgs(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}
	p := NewPipeline(cfg, sup, mtx)

	args := p.getTranscodeArgs()
	if len(args) == 0 {
		t.Fatal("expected transcode args")
	}
	// Check for key transcode settings
	found := false
	for _, a := range args {
		if a == "libx264" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected libx264 in transcode args")
	}
}
