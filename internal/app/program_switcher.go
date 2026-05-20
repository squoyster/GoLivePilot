package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/mediamtx"
)

// ProgramSwitcher handles switching the upstream source that feeds the program path.
type ProgramSwitcher interface {
	Switch(ctx context.Context, mode SourceMode) error
	CheckPersistent(ctx context.Context) error
}

type FFmpegProgramSwitcher struct {
	cfg        *config.Config
	supervisor RelaySupervisor
	mtxClient  *mediamtx.Client
}

func NewFFmpegProgramSwitcher(cfg *config.Config, supervisor RelaySupervisor, mtxClient *mediamtx.Client) *FFmpegProgramSwitcher {
	return &FFmpegProgramSwitcher{
		cfg:        cfg,
		supervisor: supervisor,
		mtxClient:  mtxClient,
	}
}

func (s *FFmpegProgramSwitcher) Switch(ctx context.Context, mode SourceMode) error {
	logger := slog.With("target_id", TargetIDProgram, "mode", mode)
	logger.Info("switching program source")

	// We use an internal path as the stable bus to ensure live/program doesn't disappear.
	// Internal Source -> live/internal-program -> [Persistent Relay] -> live/program
	internalPublishURL := s.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/internal-program"

	var input string
	var inputArgs []string
	var outputArgs []string

	if mode == SourceSlate {
		input = s.cfg.Slate.Path
		if s.cfg.Slate.StartingImage != "" {
			input = s.cfg.Slate.StartingImage
		}
		if !s.cfg.Slate.Enabled {
			return fmt.Errorf("slate is not enabled in config")
		}

		if s.cfg.Slate.Type == "image" {
			inputArgs = append(inputArgs, "-re", "-loop", "1", "-framerate", "30")
		} else if s.cfg.Slate.Type == "video" {
			inputArgs = append(inputArgs, "-re", "-stream_loop", "-1")
		}
		inputArgs = append(inputArgs, "-i", input)

		if s.cfg.Slate.Audio.Enabled && s.cfg.Slate.Audio.Type == "silent" {
			inputArgs = append(inputArgs, "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", s.cfg.Slate.Audio.SampleRate))
			outputArgs = append(outputArgs, "-map", "0:v:0", "-map", "1:a:0")
		} else {
			outputArgs = append(outputArgs, "-map", "0:v:0")
		}

		outputArgs = append(outputArgs, getTranscodeArgs()...)
	} else if mode == SourceCamera {
		input = s.cfg.Program.CameraSourceURL
		if input == "" {
			input = s.cfg.UI.CameraSourceURL
		}
		if input == "" {
			return fmt.Errorf("camera source URL is not configured")
		}

		inputArgs = append(inputArgs, "-re", "-i", input)

		outputArgs = append(outputArgs, "-fflags", "+genpts")
		outputArgs = append(outputArgs, getTranscodeArgs()...)
		outputArgs = append(outputArgs, "-avoid_negative_ts", "make_zero")
	} else if mode == SourceEnded {
		input = s.cfg.Slate.EndedImage
		if input == "" {
			return fmt.Errorf("ended slate image is not configured")
		}

		inputArgs = append(inputArgs, "-re", "-loop", "1", "-framerate", "30", "-i", input)

		if s.cfg.Slate.Audio.Enabled && s.cfg.Slate.Audio.Type == "silent" {
			inputArgs = append(inputArgs, "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", s.cfg.Slate.Audio.SampleRate))
			outputArgs = append(outputArgs, "-map", "0:v:0", "-map", "1:a:0")
		} else {
			outputArgs = append(outputArgs, "-map", "0:v:0")
		}

		outputArgs = append(outputArgs, getTranscodeArgs()...)
	} else {
		return fmt.Errorf("unsupported source mode for switcher: %s", mode)
	}

	publishURL := internalPublishURL
	if publishURL == "" {
		publishURL = s.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/program"
	}

	req := ffmpeg.StartRequest{
		TargetID:   TargetIDProgram,
		Label:      "Program Source",
		Mode:       "source",
		Binary:     s.cfg.FFmpeg.Binary,
		LogLevel:   s.cfg.FFmpeg.LogLevel,
		Input:      input,
		Output:     publishURL,
		InputArgs:  inputArgs,
		OutputArgs: outputArgs,
	}

	// Use Switch to allow seamless-ish replacement of the upstream source
	req.TargetID = "__internal_source__"
	req.Label = "Internal Source"
	if err := s.supervisor.Switch(ctx, req); err != nil {
		logger.Error("failed to switch internal source", "error", err)
		return err
	}

	// Wait for internal program source to be ready BEFORE starting/checking persistent relay
	// This ensures the persistent relay doesn't fail immediately on first start
	if s.mtxClient != nil {
		path := "live/internal-program"
		slog.Info("switcher: waiting for internal program readiness", "path", path)
		if _, err := s.mtxClient.WaitForPathReady(ctx, path, 5*time.Second); err != nil {
			slog.Warn("switcher: internal program not ready yet, proceeding with relay check anyway", "error", err)
		}
	}

	// Ensure persistent program relay is running to keep live/program alive
	return s.CheckPersistent(ctx)
}

func (s *FFmpegProgramSwitcher) CheckPersistent(ctx context.Context) error {
	targetID := TargetIDProgram
	status := s.supervisor.Status()

	if st, ok := status[targetID]; ok && st.State == "running" {
		return nil
	}

	slog.Info("starting persistent program relay")
	internalSourceURL := s.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/internal-program"
	publishURL := s.cfg.Program.PublishURL
	if publishURL == "" {
		publishURL = s.cfg.MediaEngine.MediaMTX.InternalRTMPBase + "/live/program"
	}

	req := ffmpeg.StartRequest{
		TargetID: targetID,
		Label:    "Program Output",
		Mode:     "program",
		Binary:   s.cfg.FFmpeg.Binary,
		LogLevel: s.cfg.FFmpeg.LogLevel,
		Input:    internalSourceURL,
		Output:   publishURL,
		InputArgs: []string{
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "2",
			"-i", internalSourceURL,
		},
		OutputArgs: []string{
			"-c", "copy",
			"-flvflags", "no_duration_filesize",
		},
	}

	return s.supervisor.Start(ctx, req)
}

func getTranscodeArgs() []string {
	return []string{
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
	}
}
