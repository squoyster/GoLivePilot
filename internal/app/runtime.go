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
	sourceMode SourceMode
}

func NewRuntime(cfg *config.Config, supervisor RelaySupervisor) *Runtime {
	rt := &Runtime{
		cfg:        cfg,
		started:    time.Now(),
		relays:     make(map[string]RelayState),
		supervisor: supervisor,
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

	// 1. Start Program Source Relay (Slate -> live/program)
	if err := r.startProgramSource(ctx, SourceSlate); err != nil {
		return err
	}

	// 2. Start Durable Platform Relays (live/program -> Platform)
	// These only start if they are not already running.
	return r.ensurePlatformRelays(ctx)
}

func (r *Runtime) startProgramSource(ctx context.Context, mode SourceMode) error {
	logger := slog.With("target_id", TargetIDProgram, "mode", mode)

	// Stop existing program source if any
	if r.supervisor != nil {
		_ = r.supervisor.Stop(ctx, TargetIDProgram)
	}

	var input string
	var inputArgs []string
	var outputArgs []string

	if mode == SourceSlate {
		input = r.cfg.Slate.Path
		if r.cfg.Slate.StartingImage != "" {
			input = r.cfg.Slate.StartingImage
		}
		if !r.cfg.Slate.Enabled {
			return fmt.Errorf("slate is not enabled in config")
		}

		if r.cfg.Slate.Type == "image" {
			inputArgs = append(inputArgs, "-re", "-loop", "1", "-framerate", "30")
		} else if r.cfg.Slate.Type == "video" {
			inputArgs = append(inputArgs, "-re", "-stream_loop", "-1")
		}
		inputArgs = append(inputArgs, "-i", input)

		if r.cfg.Slate.Audio.Enabled && r.cfg.Slate.Audio.Type == "silent" {
			inputArgs = append(inputArgs, "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", r.cfg.Slate.Audio.SampleRate))
			outputArgs = append(outputArgs, "-map", "0:v:0", "-map", "1:a:0")
		} else {
			outputArgs = append(outputArgs, "-map", "0:v:0")
		}

		// Use normalized transcode parameters for program output
		outputArgs = append(outputArgs,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-pix_fmt", "yuv420p",
			"-r", "30",
			"-g", "60",
			"-b:v", "2500k",
			"-maxrate", "2500k",
			"-bufsize", "5000k",
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "48000",
			"-ac", "2",
			"-flvflags", "no_duration_filesize",
			"-f", "flv",
		)
	} else if mode == SourceCamera {
		input = r.cfg.Program.CameraSourceURL
		if input == "" {
			input = r.cfg.UI.CameraSourceURL
		}
		if input == "" {
			return fmt.Errorf("camera source URL is not configured")
		}

		inputArgs = append(inputArgs, "-re", "-i", input)

		// Transcode camera to program for stability
		outputArgs = append(outputArgs,
			"-fflags", "+genpts",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-pix_fmt", "yuv420p",
			"-r", "30",
			"-g", "60",
			"-b:v", "2500k",
			"-maxrate", "2500k",
			"-bufsize", "5000k",
			"-c:a", "aac",
			"-b:a", "128k",
			"-ar", "48000",
			"-ac", "2",
			"-avoid_negative_ts", "make_zero",
			"-flvflags", "no_duration_filesize",
			"-f", "flv",
		)
	}

	publishURL := r.cfg.Program.PublishURL
	if publishURL == "" {
		// Fallback to media engine base if not set
		publishURL = r.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/program"
	}

	req := ffmpeg.StartRequest{
		TargetID:   TargetIDProgram,
		Label:      "Program Source",
		Mode:       string(mode),
		Binary:     r.cfg.FFmpeg.Binary,
		LogLevel:   r.cfg.FFmpeg.LogLevel,
		Input:      input,
		Output:     publishURL,
		InputArgs:  inputArgs,
		OutputArgs: outputArgs,
	}

	logger.Info("starting program source relay", "input", input, "output", publishURL)
	return r.supervisor.Start(ctx, req)
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

	r.sourceMode = SourceCamera

	// 1. Switch Program Source to Camera (Platform relays remain running)
	logger.Info("switching program source to camera")
	if err := r.startProgramSource(ctx, SourceCamera); err != nil {
		return err
	}

	// 2. Ensure platform relays are running (in case some didn't start in preview)
	return r.ensurePlatformRelays(ctx)
}

func (r *Runtime) StopAll() {
	slog.Info("stopping all relays")
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
