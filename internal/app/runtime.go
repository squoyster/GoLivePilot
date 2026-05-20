package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
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

	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	attempts map[string]int
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
		attempts:   make(map[string]int),
	}

	for _, t := range cfg.Targets {
		rt.relays[t.ID] = RelayState{
			TargetID: t.ID,
			State:    "stopped",
		}
	}

	return rt
}

// StartBackgroundRestarter starts a background goroutine that monitors and restarts failed relays.
func (r *Runtime) StartBackgroundRestarter(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.running = true
	r.mu.Unlock()

	go r.restarterLoop()
}

func (r *Runtime) restarterLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.checkAndRestartRelays()
		}
	}
}

func (r *Runtime) checkAndRestartRelays() {
	// We only want to restart relays if we are in a state that expects them to be running.
	if r.sourceMode == SourceStandby || r.sourceMode == SourceStopped {
		return
	}

	status := r.supervisor.Status()

	// 1. Check persistent program relay
	if st, ok := status[TargetIDProgram]; ok && st.State == ffmpeg.StateFailed {
		slog.Warn("background restarter: program relay failed, attempting restart", "target_id", TargetIDProgram)
		if err := r.switcher.CheckPersistent(r.ctx); err != nil {
			slog.Error("background restarter: failed to restart program relay", "error", err)
		}
	}

	// 2. Check platform relays
	// Only restart platform relays if the program output is ready.
	programPath := "live/program"
	pathInfo, err := r.mtxClient.GetPath(r.ctx, programPath)
	if err != nil || pathInfo == nil || !pathInfo.Ready {
		return
	}

	// Re-run ensurePlatformRelays which is idempotent
	if err := r.ensurePlatformRelays(r.ctx); err != nil {
		slog.Error("background restarter: failed to ensure platform relays", "error", err)
	}
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

	// 1. Switch Internal Source to Slate
	if err := r.switcher.Switch(ctx, SourceSlate); err != nil {
		return err
	}

	// 2. Ensure Persistent Program Relay is initiated
	// It might fail immediately if MediaMTX isn't ready, but the background restarter will pick it up.
	if err := r.switcher.CheckPersistent(ctx); err != nil {
		logger.Warn("initial program relay start failed; background restarter will retry", "error", err)
	}

	// 3. Initiate Platform Relays
	// Again, these might fail if source isn't ready, but restarter handles it.
	if err := r.ensurePlatformRelays(ctx); err != nil {
		logger.Warn("initial platform relay start failed; background restarter will retry", "error", err)
	}

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

	r.sourceMode = SourceCamera

	// 1. Switch Internal Source to Camera
	logger.Info("internal source switching: slate -> camera")
	if err := r.switcher.Switch(ctx, SourceCamera); err != nil {
		return err
	}

	// 2. Ensure Persistent Program Relay is running
	if err := r.switcher.CheckPersistent(ctx); err != nil {
		logger.Warn("program relay check failed during go-live", "error", err)
	}

	// 3. Ensure platform relays are healthy
	if err := r.ensurePlatformRelays(ctx); err != nil {
		logger.Warn("platform relay check failed during go-live", "error", err)
	}

	return nil
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
