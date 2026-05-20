package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
	"github.com/squoyster/golivepilot/internal/ffmpeg"
	"github.com/squoyster/golivepilot/internal/mediamtx"
)

type Pipeline struct {
	cfg        *config.Config
	supervisor RelaySupervisor
	mtxClient  *mediamtx.Client

	mu           sync.RWMutex
	currentState string
}

func NewPipeline(cfg *config.Config, supervisor RelaySupervisor, mtxClient *mediamtx.Client) *Pipeline {
	return &Pipeline{
		cfg:          cfg,
		supervisor:   supervisor,
		mtxClient:    mtxClient,
		currentState: "standby",
	}
}

func (p *Pipeline) CurrentState() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentState
}

func (p *Pipeline) Transition(ctx context.Context, targetState string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	slog.Info("pipeline: transitioning", "from", p.currentState, "to", targetState)

	// Find transition
	var transition *config.TransitionConfig
	for _, t := range p.cfg.Pipeline.Transitions {
		if t.From == p.currentState && t.To == targetState {
			transition = &t
			break
		}
	}

	// Also allow "any" to standby
	if transition == nil && targetState == "standby" {
		transition = &config.TransitionConfig{ID: "any_to_standby", From: p.currentState, To: "standby"}
	}

	if transition == nil {
		return fmt.Errorf("no transition defined from %s to %s", p.currentState, targetState)
	}

	// 1. Determine which nodes should be running in target state
	var targetNodeIDs []string
	for _, s := range p.cfg.Pipeline.States {
		if s.ID == targetState {
			targetNodeIDs = s.Nodes
			break
		}
	}

	// 2. Identify nodes to stop (those running but not in target state)
	// For now, we simple-mindedly stop nodes that are NOT in the target state.
	// EXCEPT platform relays which we want to keep persistent if they are already running.
	targetMap := make(map[string]bool)
	for _, id := range targetNodeIDs {
		targetMap[id] = true
	}

	status := p.supervisor.Status()
	for nodeID := range status {
		if !targetMap[nodeID] {
			// Special case: if we are transitioning preview -> live, DO NOT stop persistent relays
			if p.currentState == "preview" && targetState == "live" {
				if node, ok := p.findNode(nodeID); ok && (node.Kind == "relay.rtmp" || node.Kind == "relay.rtmps") {
					slog.Info("pipeline: keeping persistent relay", "node_id", nodeID)
					continue
				}
			}
			slog.Info("pipeline: stopping node", "node_id", nodeID)
			p.supervisor.Stop(ctx, nodeID)
		}
	}

	// 3. Start nodes in order of dependencies
	// For this v0.1 simplification, we assume the nodes list in StateConfig is ordered OR we do a simple dependency walk.
	// Let's do a simple dependency walk.
	started := make(map[string]bool)
	for _, id := range targetNodeIDs {
		if err := p.ensureNode(ctx, id, started); err != nil {
			return err
		}
	}

	p.currentState = targetState
	return nil
}

func (p *Pipeline) findNode(id string) (config.NodeConfig, bool) {
	for _, n := range p.cfg.Pipeline.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return config.NodeConfig{}, false
}

func (p *Pipeline) ensureNode(ctx context.Context, id string, started map[string]bool) error {
	if started[id] {
		return nil
	}

	node, ok := p.findNode(id)
	if !ok {
		return fmt.Errorf("node not found: %s", id)
	}

	// Ensure dependencies first
	for _, depID := range node.DependsOn {
		if err := p.ensureNode(ctx, depID, started); err != nil {
			return err
		}
	}

	// Start or check node
	status := p.supervisor.Status()
	if st, exists := status[id]; exists && st.State == "running" {
		slog.Debug("pipeline: node already running", "node_id", id)
		started[id] = true
		return nil
	}

	slog.Info("pipeline: starting node", "node_id", id, "kind", node.Kind)

	switch node.Kind {
	case "service":
		// External service like MediaMTX. We just check readiness.
		if id == "mediamtx" {
			if err := p.mtxClient.WaitUntilHealthy(ctx, 10*time.Second); err != nil {
				return fmt.Errorf("mediamtx not healthy: %w", err)
			}
		}
	case "source.slate", "source.rtmp", "stream.program", "relay.rtmp", "relay.rtmps":
		req, err := p.buildStartRequest(node)
		if err != nil {
			return err
		}
		if err := p.supervisor.Start(ctx, req); err != nil {
			return err
		}

		// Wait for readiness if it's a program stream
		if node.Kind == "stream.program" || node.Kind == "source.slate" || node.Kind == "source.rtmp" {
			// Extract path from output URL
			// Example: rtmp://.../live/program -> live/program
			// This is a bit hacky, should probably be in node config explicitly but let's try to derive it.
			path := "live/program" // Default for now
			if node.ID == "__internal_source__" {
				path = "live/internal-program"
			}

			timeout := 10 * time.Second
			if node.Timeout != "" {
				if d, err := time.ParseDuration(node.Timeout); err == nil {
					timeout = d
				}
			}

			slog.Info("pipeline: waiting for path readiness", "node_id", id, "path", path)
			if _, err := p.mtxClient.WaitForPathReady(ctx, path, timeout); err != nil {
				slog.Warn("pipeline: node path not ready yet, but continuing", "node_id", id, "path", path, "error", err)
			}
		}
	default:
		return fmt.Errorf("unknown node kind: %s", node.Kind)
	}

	started[id] = true
	return nil
}

func (p *Pipeline) buildStartRequest(node config.NodeConfig) (ffmpeg.StartRequest, error) {
	req := ffmpeg.StartRequest{
		TargetID: node.ID,
		Label:    node.Label,
		Binary:   p.cfg.FFmpeg.Binary,
		LogLevel: p.cfg.FFmpeg.LogLevel,
	}

	output := node.Output
	if node.OutputEnv != "" {
		val := p.cfg.GetEnv(node.OutputEnv)
		if val != "" {
			output = val
		}
	}
	if node.OutputKeyEnv != "" {
		key := p.cfg.GetEnv(node.OutputKeyEnv)
		if key != "" {
			// Append key to output
			// If output doesn't end in /, add it?
			// This depends on how it's defined in config.
			output += key
		}
	}
	req.Output = output

	// Mode
	switch node.Kind {
	case "source.slate", "source.rtmp":
		req.Mode = "source"
	case "stream.program":
		req.Mode = "program"
	case "relay.rtmp", "relay.rtmps":
		req.Mode = "relay"
	}

	// Args
	if node.Kind == "source.slate" {
		input := node.Input
		if input == "assets/starting-soon.png" && p.cfg.Slate.StartingImage != "" {
			input = p.cfg.Slate.StartingImage
		} else if input == "assets/stream-ended.png" && p.cfg.Slate.EndedImage != "" {
			input = p.cfg.Slate.EndedImage
		} else if input == "assets/standing-by.png" && p.cfg.Slate.StandbyImage != "" {
			input = p.cfg.Slate.StandbyImage
		}

		req.Input = input
		req.InputArgs = []string{"-re", "-loop", "1", "-framerate", "30", "-i", input}
		if p.cfg.Slate.Audio.Enabled {
			req.InputArgs = append(req.InputArgs, "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=%d", p.cfg.Slate.Audio.SampleRate))
			req.OutputArgs = append(req.OutputArgs, "-map", "0:v:0", "-map", "1:a:0")
		} else {
			req.OutputArgs = append(req.OutputArgs, "-map", "0:v:0")
		}
		req.OutputArgs = append(req.OutputArgs, p.getTranscodeArgs()...)
	} else if node.Kind == "source.rtmp" {
		req.Input = node.Input
		req.InputArgs = []string{"-re", "-i", node.Input}
		req.OutputArgs = []string{"-fflags", "+genpts"}
		req.OutputArgs = append(req.OutputArgs, p.getTranscodeArgs()...)
		req.OutputArgs = append(req.OutputArgs, "-avoid_negative_ts", "make_zero")
	} else if node.Kind == "stream.program" {
		req.Input = node.Input
		req.InputArgs = []string{"-i", node.Input}
		req.OutputArgs = append(req.OutputArgs, p.getTranscodeArgs()...)
	} else if node.Kind == "relay.rtmp" || node.Kind == "relay.rtmps" {
		req.Input = node.Input
		req.InputArgs = []string{"-i", node.Input}
		if node.Kind == "relay.rtmps" {
			// Use re-encode for RTMPS (Facebook requirement)
			req.OutputArgs = append(req.OutputArgs, p.getTranscodeArgs()...)
		} else {
			req.OutputArgs = []string{"-c", "copy"}
		}
		req.OutputArgs = append(req.OutputArgs, "-flvflags", "no_duration_filesize")
	}

	return req, nil
}

func (p *Pipeline) getTranscodeArgs() []string {
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
