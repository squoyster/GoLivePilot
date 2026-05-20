package streamswitch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// OutputStream writes FLV to an FFmpeg subprocess that publishes to a target.
// The output FFmpeg persists for the entire session and is never restarted.
type OutputStream struct {
	cfg    OutputConfig
	logger *slog.Logger

	cmd       *exec.Cmd
	stdin     io.WriteCloser
	flvWriter *FLVWriter

	mu          sync.Mutex
	running     bool
	ptsOffset   uint32 // Added to all incoming timestamps for continuity
	lastPTS     uint32 // Last written PTS
	hasAudio    bool
	hasVideo    bool
	seqSentAVC  bool
	seqSentAAC  bool
}

// NewOutputStream creates a new output stream.
func NewOutputStream(cfg OutputConfig) *OutputStream {
	return &OutputStream{
		cfg:    cfg,
		logger: slog.With("output", cfg.TargetID),
	}
}

// Start launches the FFmpeg subprocess that receives FLV on stdin.
func (out *OutputStream) Start(ctx context.Context) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if out.running {
		return fmt.Errorf("output %s already started", out.cfg.TargetID)
	}

	args := []string{"-i", "pipe:0"}
	if len(out.cfg.EncodeArgs) > 0 {
		args = append(args, out.cfg.EncodeArgs...)
	} else {
		args = append(args, "-c", "copy")
	}
	args = append(args, "-f", "flv", out.cfg.URL)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin pipe: %w", err)
	}

	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	out.cmd = cmd
	out.stdin = stdin
	out.flvWriter = NewFLVWriter(stdin)
	out.running = true

	out.logger.Info("output started", "url", out.cfg.URL)

	// Monitor process health
	go out.monitor(ctx)

	return nil
}

func (out *OutputStream) monitor(ctx context.Context) {
	done := make(chan error, 1)
	go func() {
		done <- out.cmd.Wait()
	}()

	select {
	case err := <-done:
		out.logger.Error("output process exited", "error", err)
		out.mu.Lock()
		out.running = false
		out.mu.Unlock()
	case <-ctx.Done():
		_ = out.cmd.Process.Kill()
	}
}

// WriteTag writes a single FLV tag to the output, applying PTS offset.
func (out *OutputStream) WriteTag(tag *FLVTag) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if !out.running {
		return fmt.Errorf("output %s not running", out.cfg.TargetID)
	}

	// Apply PTS offset for continuity
	adjusted := *tag
	adjusted.Timestamp += out.ptsOffset

	if err := out.flvWriter.WriteTag(&adjusted); err != nil {
		return err
	}
	out.lastPTS = adjusted.Timestamp
	return nil
}

// WriteHeader writes the FLV header to the output.
func (out *OutputStream) WriteHeader(hasAudio, hasVideo bool) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if !out.running {
		return fmt.Errorf("output %s not running", out.cfg.TargetID)
	}
	out.hasAudio = hasAudio
	out.hasVideo = hasVideo
	return out.flvWriter.WriteHeader(hasAudio, hasVideo)
}

// FlushTags writes a batch of tags to the output, all with the same PTS offset.
func (out *OutputStream) FlushTags(tags []FLVTag, ptsOffset uint32) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if !out.running {
		return fmt.Errorf("output %s not running", out.cfg.TargetID)
	}

	for i := range tags {
		t := tags[i]
		t.Timestamp += ptsOffset
		if err := out.flvWriter.WriteTag(&t); err != nil {
			return err
		}
		out.lastPTS = t.Timestamp
	}
	return nil
}

// WriteRaw writes raw bytes to the output pipe (for codec sequence headers).
func (out *OutputStream) WriteRaw(data []byte) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if !out.running {
		return fmt.Errorf("output %s not running", out.cfg.TargetID)
	}
	_, err := out.stdin.Write(data)
	return err
}

// SetPTSOffset sets the timestamp offset for all subsequent tags.
// This is called during InputSwitch to ensure continuous timestamps.
func (out *OutputStream) SetPTSOffset(offset uint32) {
	out.mu.Lock()
	defer out.mu.Unlock()
	out.ptsOffset = offset
}

// PTSOffset returns the current PTS offset.
func (out *OutputStream) PTSOffset() uint32 {
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.ptsOffset
}

// LastPTS returns the last written PTS (adjusted).
func (out *OutputStream) LastPTS() uint32 {
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.lastPTS
}

// IsRunning returns whether the output process is healthy.
func (out *OutputStream) IsRunning() bool {
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.running
}

// Close shuts down the output stream.
func (out *OutputStream) Close() error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if out.stdin != nil {
		_ = out.stdin.Close()
	}
	if out.cmd != nil && out.cmd.Process != nil {
		_ = out.cmd.Process.Kill()
	}
	out.running = false
	return nil
}

// WriteSilence injects AAC silent frames into the output stream.
// duration is in milliseconds. Minimum resolution is ~21ms (one AAC frame).
func (out *OutputStream) WriteSilence(timestamp uint32, duration time.Duration) error {
	frames := int(duration.Milliseconds()/21 + 1)
	if frames > 100 {
		frames = 100
	}
	return out.flvWriter.WriteSilentAAC(timestamp, frames)
}

// WriteGOP writes a complete GOP to the output, applying the current PTS offset.
// If force is true, codec sequence headers are resent before the GOP data.
func (out *OutputStream) WriteGOP(gop GOPBuffer, ptsOffset uint32, forceSeq bool) error {
	out.mu.Lock()
	defer out.mu.Unlock()

	if !out.running {
		return fmt.Errorf("output %s not running", out.cfg.TargetID)
	}

	for i := range gop.Tags {
		t := gop.Tags[i]
		t.Timestamp += ptsOffset
		if err := out.flvWriter.WriteTag(&t); err != nil {
			return err
		}
		out.lastPTS = t.Timestamp
	}
	return nil
}