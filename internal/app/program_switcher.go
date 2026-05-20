package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
)

// ProgramSwitcher handles switching the upstream source that feeds the program path.
type ProgramSwitcher interface {
	Switch(ctx context.Context, mode SourceMode) error
}

type FFmpegProgramSwitcher struct {
	cfg        *config.Config
	supervisor RelaySupervisor
}

func NewFFmpegProgramSwitcher(cfg *config.Config, supervisor RelaySupervisor) *FFmpegProgramSwitcher {
	return &FFmpegProgramSwitcher{
		cfg:        cfg,
		supervisor: supervisor,
	}
}

func (s *FFmpegProgramSwitcher) Switch(ctx context.Context, mode SourceMode) error {
	logger := slog.With("target_id", TargetIDProgram, "mode", mode)
	logger.Info("switching program source")

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

	publishURL := s.cfg.Program.PublishURL
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

	// Use Switch to allow seamless-ish replacement if supported by supervisor
	if err := s.supervisor.Switch(ctx, req); err != nil {
		logger.Error("failed to switch program source", "error", err)
		return err
	}

	return nil
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
