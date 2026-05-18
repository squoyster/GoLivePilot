package app

import (
	"context"
	"fmt"
	"os"
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
	var target *config.TargetConfig
	for i := range r.cfg.Targets {
		if r.cfg.Targets[i].Enabled {
			target = &r.cfg.Targets[i]
			break
		}
	}

	if target == nil {
		return fmt.Errorf("no enabled targets found")
	}

	rtmpsURL := os.Getenv(target.RTMPSURLEnv)
	if rtmpsURL == "" {
		return fmt.Errorf("RTMPS URL env var %q is empty", target.RTMPSURLEnv)
	}

	input := r.cfg.Slate.Path
	if !r.cfg.Slate.Enabled {
		// Fallback or error? README says "start FFmpeg slate relay"
		return fmt.Errorf("slate is not enabled in config")
	}

	req := ffmpeg.StartRequest{
		TargetID: target.ID,
		Label:    target.Label,
		Mode:     "preview",
		Binary:   r.cfg.FFmpeg.Binary,
		LogLevel: r.cfg.FFmpeg.LogLevel,
		Input:    input,
		Output:   rtmpsURL,
	}

	// Profiles could be applied here if needed
	for _, p := range r.cfg.Profiles {
		if p.ID == target.ProfileID {
			req.Args = p.Args
			break
		}
	}

	return r.supervisor.Start(ctx, req)
}

func (r *Runtime) StopAll() {
	if r.supervisor != nil {
		r.supervisor.StopAll(context.Background())
	}
	for id := range r.relays {
		s := r.relays[id]
		s.State = "stopped"
		r.relays[id] = s
	}
}
