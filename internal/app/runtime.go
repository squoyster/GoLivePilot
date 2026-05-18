package app

import (
	"time"

	"github.com/squoyster/golivepilot/internal/config"
)

type RelayState struct {
	TargetID  string `json:"target_id"`
	State     string `json:"state"`
	LastError string `json:"last_error,omitempty"`
}

type Runtime struct {
	cfg     *config.Config
	started time.Time
	relays  map[string]RelayState
}

func NewRuntime(cfg *config.Config) *Runtime {
	rt := &Runtime{
		cfg:     cfg,
		started: time.Now(),
		relays:  make(map[string]RelayState),
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
	return map[string]any{
		"status":  "online",
		"started": r.started.Format(time.RFC3339),
		"relays":  r.relays,
	}
}

func (r *Runtime) StartRelays() {
	for id := range r.relays {
		s := r.relays[id]
		s.State = "starting"
		r.relays[id] = s
	}
}

func (r *Runtime) StopAll() {
	for id := range r.relays {
		s := r.relays[id]
		s.State = "stopped"
		r.relays[id] = s
	}
}
