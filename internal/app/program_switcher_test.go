package app

import (
	"context"
	"testing"

	"github.com/squoyster/golivepilot/internal/config"
)

type mockProgramSwitcher struct {
	lastMode SourceMode
	err      error
}

func (m *mockProgramSwitcher) Switch(ctx context.Context, mode SourceMode) error {
	m.lastMode = mode
	return m.err
}

func TestFFmpegProgramSwitcher_Switch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Slate.Enabled = true
	cfg.Slate.Path = "test.png"
	cfg.Slate.Audio.Enabled = true
	cfg.Slate.Audio.Type = "silent"

	sup := &mockSupervisor{}
	s := NewFFmpegProgramSwitcher(cfg, sup)

	ctx := context.Background()

	t.Run("Switch to Slate", func(t *testing.T) {
		err := s.Switch(ctx, SourceSlate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Switch to Camera", func(t *testing.T) {
		cfg.Program.CameraSourceURL = "rtmp://localhost/camera"
		err := s.Switch(ctx, SourceCamera)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Switch to Ended", func(t *testing.T) {
		cfg.Slate.EndedImage = "ended.png"
		err := s.Switch(ctx, SourceEnded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Unsupported mode", func(t *testing.T) {
		err := s.Switch(ctx, "invalid")
		if err == nil {
			t.Fatal("expected error for invalid mode")
		}
	})
}

func TestRuntime_WithSwitcher(t *testing.T) {
	cfg := config.DefaultConfig()
	sup := &mockSupervisor{}
	r := NewRuntime(cfg, sup)

	switcher := &mockProgramSwitcher{}
	r.switcher = switcher

	t.Run("Status reflecting mode", func(t *testing.T) {
		r.sourceMode = SourceCamera
		status := r.Status()
		if status["source_mode"] != SourceCamera {
			t.Errorf("expected source_mode %v, got %v", SourceCamera, status["source_mode"])
		}
	})
}
