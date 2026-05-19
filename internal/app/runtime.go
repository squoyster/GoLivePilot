package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/mediamtx"
)

type RelayState struct {
	TargetID  string `json:"target_id"`
	State     string `json:"state"`
	LastError string `json:"last_error,omitempty"`
}

type SourceMode string

const (
	SourceStandby SourceMode = "standby"
	SourceSlate   SourceMode = "slate"
	SourceCamera  SourceMode = "camera"
	SourceEnded   SourceMode = "ended"
	SourceStopped SourceMode = "stopped"
)

const (
	TargetIDProgram = "__program_source__"
)

type Runtime struct {
	cfg        *config.Config
	started    time.Time
	relays     map[string]RelayState
	supervisor RelaySupervisor
	switcher   ProgramSwitcher
	sourceMode SourceMode
	mtxClient  *mediamtx.Client
}

func NewRuntime(cfg *config.Config, supervisor RelaySupervisor) *Runtime {
	rt := &Runtime{
		cfg:        cfg,
		started:    time.Now(),
		relays:     make(map[string]RelayState),
		supervisor: supervisor,
		switcher:   NewFFmpegProgramSwitcher(cfg, supervisor),
		sourceMode: SourceStandby,
		mtxClient:  mediamtx.NewClient(cfg.MediaEngine.MediaMTX.APIURL),
	}

	for _, t := range cfg.Targets {
		rt.relays[t.ID] = RelayState{
			TargetID: t.ID,
			State:    "stopped",
		}
	}

	return rt
}

func (r *Runtime) Status() map[string]any {
	status := map[string]any{
		"status":      "online",
		"started":     r.started.Format(time.RFC3339),
		"source_mode": r.sourceMode,
	}

	if r.supervisor != nil {
		status["relays"] = r.supervisor.Status()
	} else {
		status["relays"] = r.relays
	}

	return status
}

func (r *Runtime) StartPreview(ctx context.Context) error {
	logger := slog.With("mode", "preview")
	logger.Debug("starting preview")

	r.sourceMode = SourceSlate

	// 1. Switch Program Source to Slate
	if err := r.switcher.Switch(ctx, SourceSlate); err != nil {
		return err
	}

	// 3. Wait for live/program readiness
	programPath := "live/program"
	logger.Info("waiting for program source readiness", "path", programPath)
	if _, err := r.mtxClient.WaitForPathReady(ctx, programPath, 10*time.Second); err != nil {
		logger.Error("program source failed to become ready", "error", err)
		return err
	}
	logger.Info("program source is ready")

	// 4. Start Durable Platform Relays (live/program -> Platform)
	return r.ensurePlatformRelays(ctx)
}

func (r *Runtime) ensurePlatformRelays(ctx context.Context) error {
	var firstErr error

	programSource := r.cfg.Program.SourceURL
	if programSource == "" {
		programSource = r.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/program"
	}

	for _, t := range r.cfg.Targets {
		if !t.Enabled {
			continue
		}

		// Check if already running
		status := r.supervisor.Status()
		if st, exists := status[t.ID]; exists && st.State == "running" {
			slog.Info("skipping platform relay because already running", "target_id", t.ID)
			continue
		} else {
			slog.Info("starting platform relay", "target_id", t.ID, "last_state", st.State)
		}

		tLogger := slog.With("target_id", t.ID, "mode", "platform-relay")

		// Resolve output URL
		targetURL := os.Getenv(t.RTMPSURLEnv)
		if targetURL == "" {
			if strings.HasPrefix(t.RTMPSURLEnv, "rtmp://") || strings.HasPrefix(t.RTMPSURLEnv, "rtmps://") {
				targetURL = t.RTMPSURLEnv
			}
		}

		if targetURL != "" && t.RTMPSKeyEnv != "" {
			key := os.Getenv(t.RTMPSKeyEnv)
			if key == "" {
				key = t.RTMPSKeyEnv
			}
			if key != "" {
				targetURL = strings.TrimSuffix(targetURL, "/") + "/" + key
			}
		}

		if targetURL == "" {
			err := fmt.Errorf("RTMPS URL env var %q is empty", t.RTMPSURLEnv)
			tLogger.Error("failed to resolve target URL", "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// Platform relay is a simple copy from program to target
		req := ffmpeg.StartRequest{
			TargetID:  t.ID,
			Label:     t.Label,
			Mode:      "relay",
			Binary:    r.cfg.FFmpeg.Binary,
			LogLevel:  r.cfg.FFmpeg.LogLevel,
			Input:     programSource,
			Output:    targetURL,
			InputArgs: []string{"-re", "-i", programSource},
			OutputArgs: []string{
				"-c", "copy",
				"-flvflags", "no_duration_filesize",
				"-f", "flv",
			},
		}

		tLogger.Info("starting platform relay", "url", targetURL)
		if err := r.supervisor.Start(ctx, req); err != nil {
			tLogger.Error("failed to start platform relay", "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func (r *Runtime) StartGoLive(ctx context.Context) error {
	logger := slog.With("mode", "go-live")
	logger.Info("switching to camera (go-live)")

	// 1. Verify camera source is ready if it's a MediaMTX path
	cameraSource := r.cfg.Program.CameraSourceURL
	if cameraSource == "" {
		cameraSource = r.cfg.UI.CameraSourceURL
	}
	if strings.Contains(cameraSource, r.cfg.MediaEngine.MediaMTX.InternalRTMPBase) {
		// It's a local MediaMTX path, try to wait for it
		parts := strings.Split(cameraSource, "/")
		if len(parts) > 0 {
			path := parts[len(parts)-1]
			// If it's live/camera, parts might be [... "live", "camera"]
			if strings.Contains(cameraSource, "/live/") {
				path = "live/" + path
			}

			logger.Info("waiting for camera source readiness", "path", path)
			if _, err := r.mtxClient.WaitForPathReady(ctx, path, 5*time.Second); err != nil {
				return fmt.Errorf("camera source %q not ready: %w", path, err)
			}
			logger.Info("camera source is ready")
		}
	}

	r.sourceMode = SourceCamera

	// 2. Switch Program Source to Camera (Platform relays remain running)
	logger.Info("switching program source to camera")
	if err := r.switcher.Switch(ctx, SourceCamera); err != nil {
		return err
	}

	// 3. Wait for live/program readiness again after switch
	// Although we want platform relays to stay running, if we didn't stop them,
	// ffmpeg copy might handle the source switch, or it might exit.
	// If it exits, ensurePlatformRelays will restart it.
	programPath := "live/program"
	if _, err := r.mtxClient.WaitForPathReady(ctx, programPath, 10*time.Second); err != nil {
		logger.Error("program source failed to become ready after switch", "error", err)
	}

	// 4. Ensure platform relays are running (in case some didn't start in preview or exited during switch)
	return r.ensurePlatformRelays(ctx)
}

func (r *Runtime) StopAll() {
	slog.Info("stopping all relays - switching to ended slate")
	r.sourceMode = SourceEnded

	// Instead of stopping everything immediately, we switch to Ended slate.
	// We'll keep the platform relays running so they pick up the ended image.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := r.switcher.Switch(ctx, SourceEnded); err != nil {
		slog.Error("failed to switch to ended program source", "error", err)
		// Fallback to hard stop if we can't show ended slate
		r.HardStop()
		return
	}

	// Wait a bit for the ended slate to propagate before we actually kill everything?
	// For now, let's keep it in "ended" state. The user can HardStop if they want to.
	// Or we can schedule a hard stop.
}

func (r *Runtime) HardStop() {
	slog.Info("hard stopping all relays")
	r.sourceMode = SourceStopped
	if r.supervisor != nil {
		r.supervisor.StopAll(context.Background())
	}
	for id := range r.relays {
		s := r.relays[id]
		s.State = "stopped"
		r.relays[id] = s
	}
}
