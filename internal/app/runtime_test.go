package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/mediamtx"
)

type mockMediaMTXClient struct {
	getPathFunc       func(ctx context.Context, path string) (*mediamtx.PathInfo, error)
	waitHealthyFunc   func(ctx context.Context, timeout time.Duration) error
	waitPathReadyFunc func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error)
}

func (m *mockMediaMTXClient) GetPath(ctx context.Context, path string) (*mediamtx.PathInfo, error) {
	if m.getPathFunc != nil {
		return m.getPathFunc(ctx, path)
	}
	return nil, errors.New("not implemented")
}

func (m *mockMediaMTXClient) WaitUntilHealthy(ctx context.Context, timeout time.Duration) error {
	if m.waitHealthyFunc != nil {
		return m.waitHealthyFunc(ctx, timeout)
	}
	return nil
}

func (m *mockMediaMTXClient) WaitForPathReady(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
	if m.waitPathReadyFunc != nil {
		return m.waitPathReadyFunc(ctx, path, timeout)
	}
	return nil, errors.New("not implemented")
}

type mockSupervisor struct {
	starts []ffmpeg.StartRequest
	stops  []string
	status map[string]ffmpeg.RelayStatus
	err    error
}

func (m *mockSupervisor) Start(ctx context.Context, req ffmpeg.StartRequest) error {
	m.starts = append(m.starts, req)
	return m.err
}

func (m *mockSupervisor) Stop(ctx context.Context, targetID string) error {
	m.stops = append(m.stops, targetID)
	return nil
}

func (m *mockSupervisor) Switch(ctx context.Context, req ffmpeg.StartRequest) error {
	m.starts = append(m.starts, req)
	return m.err
}

func (m *mockSupervisor) StopAll(ctx context.Context) {
	for id := range m.status {
		m.stops = append(m.stops, id)
	}
}

func (m *mockSupervisor) Status() map[string]ffmpeg.RelayStatus {
	if m.status != nil {
		return m.status
	}
	return make(map[string]ffmpeg.RelayStatus)
}

func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.MediaEngine.MediaMTX.APIURL = "http://localhost:9997"
	cfg.MediaEngine.MediaMTX.InternalRTMPBase = "rtmp://localhost:1935"
	cfg.Slate.Enabled = true
	cfg.Slate.Path = "test.png"
	cfg.Slate.StartingImage = "starting.png"
	cfg.Slate.EndedImage = "ended.png"
	cfg.Slate.Audio.Enabled = true
	cfg.Slate.Audio.Type = "silent"
	cfg.Slate.Audio.SampleRate = 48000
	cfg.Program.CameraSourceURL = "rtmp://localhost/camera"
	cfg.Program.SourceURL = "rtmp://localhost/program"
	return cfg
}

func TestRuntime_NewRuntime(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}

	t.Run("default creates real client", func(t *testing.T) {
		r := NewRuntime(cfg, sup)
		if r == nil {
			t.Fatal("expected runtime")
		}
		if r.mtxClient == nil {
			t.Error("expected default mtxClient")
		}
	})

	t.Run("with option overrides client", func(t *testing.T) {
		r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))
		if r.mtxClient != mtx {
			t.Error("expected injected client")
		}
	})
}

func TestRuntime_StartPreview(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
			return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
		},
	}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	err := r.StartPreview(ctx)
	if err != nil {
		t.Fatalf("StartPreview failed: %v", err)
	}
	if r.pipeline.CurrentState() != "preview" {
		t.Errorf("expected state preview, got %s", r.pipeline.CurrentState())
	}
}

func TestRuntime_StartGoLive(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
			return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
		},
	}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	_ = r.StartPreview(ctx)
	err := r.StartGoLive(ctx)
	if err != nil {
		t.Fatalf("StartGoLive failed: %v", err)
	}
	if r.pipeline.CurrentState() != "live" {
		t.Errorf("expected state live, got %s", r.pipeline.CurrentState())
	}
}

func TestRuntime_StopAll(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
			return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
		},
	}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	_ = r.StartPreview(ctx)
	r.StopAll()
	if r.pipeline.CurrentState() != "ended" {
		t.Errorf("expected state ended, got %s", r.pipeline.CurrentState())
	}
}

func TestRuntime_HardStop(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
	}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	_ = r.StartPreview(ctx)
	r.HardStop()
	if r.pipeline.CurrentState() != "standby" {
		t.Errorf("expected state standby, got %s", r.pipeline.CurrentState())
	}
}

func TestRuntime_Status(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{
		status: map[string]ffmpeg.RelayStatus{
			"test": {State: ffmpeg.StateRunning},
		},
	}
	mtx := &mockMediaMTXClient{}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	status := r.Status()
	if status["status"] != "online" {
		t.Errorf("expected status online, got %v", status["status"])
	}
	if status["source_mode"] != "standby" {
		t.Errorf("expected source_mode standby, got %v", status["source_mode"])
	}
}

func TestRuntime_DiagFacebook(t *testing.T) {
	cfg := pipelineConfig()
	cfg.Targets = []config.TargetConfig{
		{
			ID:          "facebook-main",
			Label:       "Facebook",
			Platform:    "facebook",
			Enabled:     true,
			RTMPSURLEnv: "rtmps://live-api-s.facebook.com:443/rtmp/",
			RTMPSKeyEnv: "FB-TEST-KEY",
		},
	}
	sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
	mtx := &mockMediaMTXClient{}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	logs, err := r.DiagFacebook(ctx, "facebook-main")
	_ = logs
	_ = err
}

func TestRuntime_DiagFacebook_TargetNotFound(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{}
	mtx := &mockMediaMTXClient{}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	_, err := r.DiagFacebook(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
}

func TestRuntime_BackgroundRestarter(t *testing.T) {
	t.Run("starts without panic", func(t *testing.T) {
		cfg := pipelineConfig()
		cfg.Behavior.RestarterInterval = "100ms"
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{
			waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
			waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
				return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
			},
		}
		r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

		ctx, cancel := context.WithCancel(context.Background())
		r.StartBackgroundRestarter(ctx)
		time.Sleep(50 * time.Millisecond)
		cancel()
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("no double start", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{}
		r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

		ctx, cancel := context.WithCancel(context.Background())
		r.StartBackgroundRestarter(ctx)
		r.StartBackgroundRestarter(ctx)
		cancel()
	})

	t.Run("skips when standby", func(t *testing.T) {
		cfg := pipelineConfig()
		sup := &mockSupervisor{status: map[string]ffmpeg.RelayStatus{}}
		mtx := &mockMediaMTXClient{}
		r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

		r.checkAndRestartRelays()
	})
}

func TestRuntime_CheckAndRestartRelays(t *testing.T) {
	cfg := pipelineConfig()
	sup := &mockSupervisor{
		status: map[string]ffmpeg.RelayStatus{
			"mediamtx": {State: ffmpeg.StateRunning},
		},
	}
	mtx := &mockMediaMTXClient{
		waitHealthyFunc: func(ctx context.Context, timeout time.Duration) error { return nil },
		waitPathReadyFunc: func(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error) {
			return &mediamtx.PathInfo{Ready: true, Available: true, Online: true, Tracks: []string{"0"}}, nil
		},
	}
	r := NewRuntime(cfg, sup, WithMediaMTXClient(mtx))

	ctx := context.Background()
	_ = r.pipeline.Transition(ctx, "preview")

	r.checkAndRestartRelays()
}