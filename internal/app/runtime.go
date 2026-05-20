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
	mtxClient := mediamtx.NewClient(cfg.MediaEngine.MediaMTX.APIURL)
	rt := &Runtime{
		cfg:        cfg,
		started:    time.Now(),
		relays:     make(map[string]RelayState),
		supervisor: supervisor,
		mtxClient:  mtxClient,
		switcher:   NewFFmpegProgramSwitcher(cfg, supervisor, mtxClient),
		sourceMode: SourceStandby,
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

	// 2. Wait for live/internal-program readiness (the source for the persistent relay)
	internalProgramPath := "live/internal-program"
	logger.Info("waiting for internal program source readiness", "path", internalProgramPath)
	if _, err := r.mtxClient.WaitForPathReady(ctx, internalProgramPath, 10*time.Second); err != nil {
		logger.Error("internal program source failed to become ready", "error", err)
		return err
	}

	// 3. Wait for live/program readiness (the output of the persistent relay)
	programPath := "live/program"
	logger.Info("waiting for program source readiness", "path", programPath)
	if _, err := r.mtxClient.WaitForPathReady(ctx, programPath, 10*time.Second); err != nil {
		logger.Error("program source failed to become ready", "error", err)
		return err
	}
	logger.Info("program source is ready")

	// 4. Start Durable Platform Relays (live/program -> Platform)
	if err := r.ensurePlatformRelays(ctx); err != nil {
		return err
	}

	// Log PIDs for lifecycle tracking
	status := r.supervisor.Status()
	for id, s := range status {
		if s.Mode == "relay" && s.State == "running" {
			logger.Info("platform relay PID at Preview", "target_id", id, "pid", s.PID)
		}
	}

	// Double check after a few seconds to ensure they didn't fail immediately
	go func() {
		time.Sleep(5 * time.Second)
		_ = r.ensurePlatformRelays(context.Background())
	}()

	return nil
}

func (r *Runtime) ensurePlatformRelays(ctx context.Context) error {
	var firstErr error

	programSource := r.cfg.Program.SourceURL
	if programSource == "" {
		programSource = r.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/program"
	}

	// Get current statuses from supervisor
	status := r.supervisor.Status()

	for _, t := range r.cfg.Targets {
		if !t.Enabled {
			continue
		}

		tLogger := slog.With("target_id", t.ID, "mode", "platform-relay")

		// Check if already running
		if st, exists := status[t.ID]; exists && st.State == "running" {
			tLogger.Info("platform relay already running; leaving intact")
			continue
		} else if exists {
			tLogger.Info("platform relay missing/failed; starting", "last_state", st.State)
		} else {
			tLogger.Info("platform relay missing; starting")
		}

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
		}

		if strings.Contains(strings.ToLower(t.Label), "facebook") || strings.Contains(targetURL, "facebook.com") {
			// Facebook re-encode settings for stability
			req.OutputArgs = []string{
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-tune", "zerolatency",
				"-pix_fmt", "yuv420p",
				"-r", "30",
				"-g", "60",
				"-keyint_min", "60",
				"-sc_threshold", "0",
				"-b:v", "2500k",
				"-maxrate", "2500k",
				"-bufsize", "5000k",
				"-c:a", "aac",
				"-b:a", "128k",
				"-ar", "48000",
				"-ac", "2",
				"-flvflags", "no_duration_filesize",
			}
		} else {
			// Default to copy for others if compatible
			req.OutputArgs = []string{
				"-c", "copy",
				"-flvflags", "no_duration_filesize",
			}
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

	// Log existing PIDs before switch
	statusBefore := r.supervisor.Status()
	for id, s := range statusBefore {
		if s.Mode == "relay" && s.State == "running" {
			logger.Info("platform relay PID before Go Live switch", "target_id", id, "pid", s.PID)
		}
	}

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
	logger.Info("program source switching: slate -> camera")
	if err := r.switcher.Switch(ctx, SourceCamera); err != nil {
		return err
	}

	// 2.5 Wait for live/internal-program readiness again after switch
	internalProgramPath := "live/internal-program"
	if _, err := r.mtxClient.WaitForPathReady(ctx, internalProgramPath, 10*time.Second); err != nil {
		logger.Error("internal program source failed to become ready after switch", "error", err)
	}

	// 3. Wait for live/program readiness again after switch
	programPath := "live/program"
	if _, err := r.mtxClient.WaitForPathReady(ctx, programPath, 10*time.Second); err != nil {
		logger.Error("program source failed to become ready after switch", "error", err)
	}

	// 4. Ensure platform relays are running (only if they died during switch)
	logger.Info("ensuring platform relays are healthy after switch")
	// Small delay to allow platform relays that failed due to the switch to actually exit and be detected as stopped
	time.Sleep(2 * time.Second)
	err := r.ensurePlatformRelays(ctx)

	// Log PIDs after switch for verification
	statusAfter := r.supervisor.Status()
	for id, s := range statusAfter {
		if s.Mode == "relay" && s.State == "running" {
			logger.Info("platform relay PID after Go Live switch", "target_id", id, "pid", s.PID)
			if old, ok := statusBefore[id]; ok && old.State == "running" && old.PID != s.PID {
				logger.Error("LIFECYCLE BUG: platform relay PID changed during Go Live", "target_id", id, "old_pid", old.PID, "new_pid", s.PID)
			}
		}
	}

	return err
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

func (r *Runtime) DiagFacebook(ctx context.Context, targetID string) ([]string, error) {
	logger := slog.With("target_id", targetID, "mode", "diag")
	logger.Info("starting facebook diagnostic")

	var target *config.TargetConfig
	for _, t := range r.cfg.Targets {
		if t.ID == targetID {
			target = &t
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("target %q not found", targetID)
	}

	// Resolve output URL
	targetURL := os.Getenv(target.RTMPSURLEnv)
	if targetURL == "" {
		if strings.HasPrefix(target.RTMPSURLEnv, "rtmp") {
			targetURL = target.RTMPSURLEnv
		}
	}
	if targetURL != "" && target.RTMPSKeyEnv != "" {
		key := os.Getenv(target.RTMPSKeyEnv)
		if key == "" {
			key = target.RTMPSKeyEnv
		}
		if key != "" {
			targetURL = strings.TrimSuffix(targetURL, "/") + "/" + key
		}
	}

	if targetURL == "" {
		return nil, fmt.Errorf("failed to resolve target URL for %q", targetID)
	}

	// Use slate image as source for test
	input := r.cfg.Slate.Path
	if r.cfg.Slate.StartingImage != "" {
		input = r.cfg.Slate.StartingImage
	}

	diagID := "__diag_facebook__"
	req := ffmpeg.StartRequest{
		TargetID:  diagID,
		Label:     "Facebook Diagnostic",
		Mode:      "diag",
		Binary:    r.cfg.FFmpeg.Binary,
		LogLevel:  "info",
		Input:     input,
		Output:    targetURL,
		InputArgs: []string{"-re", "-loop", "1", "-framerate", "30", "-i", input},
		OutputArgs: []string{
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-pix_fmt", "yuv420p",
			"-t", "10", // Run for only 10 seconds
			"-f", "flv",
		},
	}

	if err := r.supervisor.Start(ctx, req); err != nil {
		return nil, err
	}

	// Wait for it to finish or timeout
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			_ = r.supervisor.Stop(context.Background(), diagID)
			return nil, fmt.Errorf("diagnostic timed out")
		case <-ticker.C:
			status := r.supervisor.Status()
			if st, ok := status[diagID]; ok {
				if st.State == "stopped" || st.State == "failed" {
					if st.State == "failed" {
						return st.Logs, fmt.Errorf("diagnostic failed: %s", st.LastError)
					}
					return st.Logs, nil
				}
			} else {
				// If it disappeared from supervisor, check last status
				return nil, fmt.Errorf("diagnostic process disappeared")
			}
		}
	}
}
