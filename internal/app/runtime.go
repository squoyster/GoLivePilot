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
	"github.com/squoyster/golivepilot/internal/streamswitch"
)

type Runtime struct {
	cfg        *config.Config
	started    time.Time
	supervisor RelaySupervisor
	pipeline   *Pipeline
	mtxClient  *mediamtx.Client
	switcher   *streamswitch.StreamSwitch

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	busMode bool // when true, StreamSwitch handles the program bus
}

// RuntimeOption configures a Runtime.
type RuntimeOption func(*Runtime)

// WithMediaMTXClient overrides the default MediaMTX client (useful for testing).
func WithMediaMTXClient(c *mediamtx.Client) RuntimeOption {
	return func(r *Runtime) {
		r.mtxClient = c
	}
}

func NewRuntime(cfg *config.Config, supervisor RelaySupervisor, opts ...RuntimeOption) *Runtime {
	rt := &Runtime{
		cfg:        cfg,
		started:    time.Now(),
		supervisor: supervisor,
	}

	for _, opt := range opts {
		opt(rt)
	}

	if rt.mtxClient == nil {
		rt.mtxClient = mediamtx.NewClient(cfg.MediaEngine.MediaMTX.APIURL)
	}

	rt.pipeline = NewPipeline(cfg, supervisor, rt.mtxClient)

	rt.initStreamSwitch(cfg)

	return rt
}

// initStreamSwitch creates a StreamSwitch from pipeline config sources and targets.
func (r *Runtime) initStreamSwitch(cfg *config.Config) {
	// Build input and output configs from the pipeline nodes
	var inputs []streamswitch.InputConfig
	var outputs []streamswitch.OutputConfig
	foundBus := false

	for _, node := range cfg.Pipeline.Nodes {
		if node.Kind == "bus" {
			foundBus = true
			// Inputs are provided as comma-separated IDs in the input field
			// We look up their FFmpeg args from the defined nodes
			for _, sid := range node.SourceIDs {
				for _, n := range cfg.Pipeline.Nodes {
					if n.ID == sid {
						inputs = append(inputs, streamswitch.InputConfig{
							ID:   streamswitch.SourceID(sid),
							Args: buildSwitchArgs(&n, cfg),
						})
					}
				}
			}
			// Outputs match relay-type nodes that depend on this bus
			for _, n := range cfg.Pipeline.Nodes {
				for _, dep := range n.DependsOn {
					if dep == node.ID && (n.Kind == "relay.rtmp" || n.Kind == "relay.rtmps") {
						url := resolveNodeOutput(&n, cfg)
						if url != "" {
							var encodeArgs []string
							if n.Kind == "relay.rtmps" {
								encodeArgs = getTranscodeArgs()
							}
							outputs = append(outputs, streamswitch.OutputConfig{
								TargetID:   n.ID,
								URL:        url,
								EncodeArgs: encodeArgs,
							})
						}
					}
				}
			}
		}
	}

	if !foundBus || len(inputs) == 0 || len(outputs) == 0 {
		return
	}

	sw, err := streamswitch.NewStreamSwitch(streamswitch.Config{
		Inputs:  inputs,
		Outputs: outputs,
	})
	if err != nil {
		slog.Warn("failed to create stream switch", "error", err)
		return
	}

	r.switcher = sw
	r.busMode = true
	slog.Info("stream switch initialized",
		"inputs", len(inputs),
		"outputs", len(outputs),
	)
}

// buildSwitchArgs constructs FFmpeg args for a source node.
func buildSwitchArgs(node *config.NodeConfig, cfg *config.Config) []string {
	switch node.Kind {
	case "source.slate":
		args := []string{
			"-re", "-loop", "1", "-framerate", "30", "-i", node.Input,
		}
		if cfg.Slate.Audio.Enabled {
			args = append(args, "-f", "lavfi", "-i",
				fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", cfg.Slate.Audio.SampleRate))
		}
		args = append(args, "-c:v", "libx264", "-preset", "veryfast",
			"-tune", "stillimage", "-pix_fmt", "yuv420p",
			"-r", "30", "-g", "60",
			"-b:v", "2500k", "-maxrate", "2500k", "-bufsize", "5000k",
			"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
			"-f", "flv",
		)
		return args
	case "source.rtmp":
		return []string{
			"-re", "-i", node.Input,
			"-c:v", "libx264", "-preset", "veryfast",
			"-pix_fmt", "yuv420p",
			"-r", "30", "-g", "60",
			"-b:v", "2500k", "-maxrate", "2500k", "-bufsize", "5000k",
			"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
			"-f", "flv",
		}
	default:
		return nil
	}
}

// resolveNodeOutput resolves the output URL for a relay node.
func resolveNodeOutput(node *config.NodeConfig, cfg *config.Config) string {
	out := node.Output
	if node.OutputEnv != "" {
		val := cfg.GetEnv(node.OutputEnv)
		if val != "" {
			out = val
		} else if strings.HasPrefix(node.OutputEnv, "rtmp://") || strings.HasPrefix(node.OutputEnv, "rtmps://") {
			out = node.OutputEnv
		}
	}
	if node.OutputKeyEnv != "" {
		key := cfg.GetEnv(node.OutputKeyEnv)
		if key == "" {
			key = node.OutputKeyEnv
		}
		if key != "" {
			out = strings.TrimSuffix(out, "/") + "/" + key
		}
	}
	return out
}

func getTranscodeArgs() []string {
	return []string{
		"-vf", "scale='trunc(iw/2)*2:trunc(ih/2)*2'",
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
	interval := 5 * time.Second
	if r.cfg.Behavior.RestarterInterval != "" {
		if d, err := time.ParseDuration(r.cfg.Behavior.RestarterInterval); err == nil {
			interval = d
		}
	}

	ticker := time.NewTicker(interval)
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
	state := r.pipeline.CurrentState()
	if state == "standby" || state == "ended" {
		return
	}

	// Re-ensure nodes for current state
	var targetNodeIDs []string
	for _, s := range r.cfg.Pipeline.States {
		if s.ID == state {
			targetNodeIDs = s.Nodes
			break
		}
	}

	started := make(map[string]bool)
	for _, id := range targetNodeIDs {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = r.pipeline.ensureNode(checkCtx, id, started)
		checkCancel()
	}
}

func (r *Runtime) Status() map[string]any {
	status := map[string]any{
		"status":      "online",
		"started":     r.started.Format(time.RFC3339),
		"source_mode": r.pipeline.CurrentState(),
	}

	if r.supervisor != nil {
		status["relays"] = r.supervisor.Status()
	}

	return status
}

func (r *Runtime) StartPreview(ctx context.Context) error {
	if r.busMode && r.switcher != nil {
		if err := r.switcher.Start(ctx); err != nil {
			return err
		}
		if err := r.switcher.Switch(ctx, streamswitch.SourceSlate); err != nil {
			return err
		}
	}
	if err := r.pipeline.Transition(ctx, "preview"); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) StartGoLive(ctx context.Context) error {
	if r.busMode && r.switcher != nil {
		if err := r.switcher.Switch(ctx, streamswitch.SourceCamera); err != nil {
			return err
		}
	}
	if err := r.pipeline.Transition(ctx, "live"); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) StopAll() {
	if r.busMode && r.switcher != nil {
		r.switcher.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := r.pipeline.Transition(ctx, "ended"); err != nil {
		slog.Error("pipeline: failed to transition to ended", "error", err)
		r.HardStop()
	}
}

func (r *Runtime) HardStop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.pipeline.Transition(ctx, "standby"); err != nil {
		slog.Error("pipeline: failed to transition to standby", "error", err)
		if r.supervisor != nil {
			r.supervisor.StopAll(context.Background())
		}
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
				return nil, fmt.Errorf("diagnostic process disappeared")
			}
		}
	}
}