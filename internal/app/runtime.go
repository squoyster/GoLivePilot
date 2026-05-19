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
	SourceInit   SourceMode = "initialized"
	SourceSlate  SourceMode = "slate"
	SourceCamera SourceMode = "camera"
	SourceNone   SourceMode = "none"
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
		sourceMode: SourceInit,
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

	// Stop any existing relays before starting slate preview
	if r.supervisor != nil {
		r.supervisor.StopAll(ctx)
	}

	input := r.cfg.Slate.Path
	if !r.cfg.Slate.Enabled {
		err := fmt.Errorf("slate is not enabled in config")
		logger.Error("preview failed", "error", err)
		return err
	}

	// Slate needs to loop if it's an image or short video
	inputArgs := []string{}
	outputArgs := []string{}

	// Finalize Input 0 (Video Slate)
	// We use the same parameters for both local preview and external target slate relays
	// to ensure consistency and because they have been proven stable.

	// Input 0: Image/Video Slate
	if r.cfg.Slate.Type == "image" {
		inputArgs = append(inputArgs, "-re", "-loop", "1", "-framerate", "30", "-i", input)
	} else if r.cfg.Slate.Type == "video" {
		inputArgs = append(inputArgs, "-re", "-stream_loop", "-1", "-i", input)
	} else {
		inputArgs = append(inputArgs, "-i", input)
	}

	// Input 1: Silent Audio
	if r.cfg.Slate.Audio.Enabled && r.cfg.Slate.Audio.Type == "silent" {
		inputArgs = append(inputArgs, "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", r.cfg.Slate.Audio.SampleRate))
		outputArgs = append(outputArgs, "-map", "0:v:0", "-map", "1:a:0")
	} else {
		outputArgs = append(outputArgs, "-map", "0:v:0")
	}

	// Stable encoding parameters for slate mode
	outputArgs = append(outputArgs,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "stillimage",
		"-pix_fmt", "yuv420p",
		"-r", "30",
		"-g", "60",
		"-b:v", "3000k",
		"-maxrate", "3000k",
		"-bufsize", "6000k",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "48000",
		"-ac", "2",
	)

	// Note: We deliberately do NOT append profile args here for slate mode
	// to avoid duplicate/conflicting codec and format options.

	// 1. Start relays for ALL enabled targets
	// This includes our special "__preview__" target if it's in the config.

	var firstErr error

	for _, t := range r.cfg.Targets {
		if !t.Enabled {
			continue
		}

		// Skip if target doesn't support preview
		if t.Lifecycle.SupportsPreview == false && t.ID != "__preview__" {
			continue
		}

		tLogger := slog.With("target_id", t.ID, "mode", "preview")

		// Resolve output URL
		targetURL := os.Getenv(t.RTMPSURLEnv)
		if targetURL == "" {
			// Support direct URLs if they look like RTMP/S
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
			tLogger.Error("preview failed for target", "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		tLogger.Info("starting slate relay", "url", targetURL)

		req := ffmpeg.StartRequest{
			TargetID:   t.ID,
			Label:      t.Label,
			Mode:       "preview",
			Binary:     r.cfg.FFmpeg.Binary,
			LogLevel:   r.cfg.FFmpeg.LogLevel,
			Input:      input,
			Output:     targetURL,
			InputArgs:  inputArgs,
			OutputArgs: outputArgs,
		}

		if err := r.supervisor.Start(ctx, req); err != nil {
			tLogger.Error("failed to start slate relay", "error", err)
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

	cameraURL := r.cfg.UI.CameraSourceURL
	if cameraURL == "" {
		err := fmt.Errorf("ui.camera_source_url is not configured")
		logger.Error("go-live failed", "error", err)
		return err
	}

	// Stop existing relays (e.g. slate preview) before switching to camera
	if r.supervisor != nil {
		r.supervisor.StopAll(ctx)
	}

	// For v0.1, we use a simple stream copy from camera source to preview output.
	// However, copying from an RTMP/WebRTC source to RTMPS/HLS targets can be flaky
	// if the source doesn't have a stable GOP or compatible codecs.
	// We'll use the transcode profile if configured, otherwise fallback to copy.
	inputArgs := []string{"-re", "-i", cameraURL}
	outputArgs := []string{}

	// TODO: For now we'll force transcode for go-live to ensure stability,
	// similar to how we do it for slate.
	outputArgs = append(outputArgs,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "60",
		"-b:v", "3000k",
		"-maxrate", "3000k",
		"-bufsize", "6000k",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "48000",
		"-ac", "2",
	)

	var firstErr error

	for _, t := range r.cfg.Targets {
		if !t.Enabled {
			continue
		}

		// Skip if target doesn't support go-live
		if t.Lifecycle.SupportsGoLive == false && t.ID != "__preview__" {
			continue
		}

		tLogger := slog.With("target_id", t.ID, "mode", "go-live")

		// Resolve output URL
		targetURL := os.Getenv(t.RTMPSURLEnv)
		if targetURL == "" {
			// Support direct URLs if they look like RTMP/S
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
			tLogger.Error("go-live failed for target", "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		tLogger.Info("starting camera relay", "url", targetURL)

		req := ffmpeg.StartRequest{
			TargetID:   t.ID,
			Label:      t.Label,
			Mode:       "go-live",
			Binary:     r.cfg.FFmpeg.Binary,
			LogLevel:   r.cfg.FFmpeg.LogLevel,
			Input:      cameraURL,
			Output:     targetURL,
			InputArgs:  inputArgs,
			OutputArgs: outputArgs,
		}

		if err := r.supervisor.Start(ctx, req); err != nil {
			tLogger.Error("failed to start camera relay", "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func (r *Runtime) StopAll() {
	slog.Info("stopping all relays")
	r.sourceMode = SourceNone
	if r.supervisor != nil {
		r.supervisor.StopAll(context.Background())
	}
	for id := range r.relays {
		s := r.relays[id]
		s.State = "stopped"
		r.relays[id] = s
	}
}
