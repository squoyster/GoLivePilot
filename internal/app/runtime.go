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

type Runtime struct {
	cfg        *config.Config
	started    time.Time
	relays     map[string]RelayState
	supervisor RelaySupervisor
}

func NewRuntime(cfg *config.Config, supervisor RelaySupervisor) *Runtime {
	rt := &Runtime{
		cfg:        cfg,
		started:    time.Now(),
		relays:     make(map[string]RelayState),
		supervisor: supervisor,
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
		"status":  "online",
		"started": r.started.Format(time.RFC3339),
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
			if strings.HasPrefix(t.RTMPSURLEnv, "rtmp://") || strings.HasPrefix(t.RTMPSURLEnv, "rtmps://") {
				targetURL = t.RTMPSURLEnv
			} else {
				err := fmt.Errorf("RTMPS URL env var %q is empty", t.RTMPSURLEnv)
				tLogger.Error("preview failed for target", "error", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
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

func (r *Runtime) StopAll() {
	slog.Info("stopping all relays")
	if r.supervisor != nil {
		r.supervisor.StopAll(context.Background())
	}
	for id := range r.relays {
		s := r.relays[id]
		s.State = "stopped"
		r.relays[id] = s
	}
}
