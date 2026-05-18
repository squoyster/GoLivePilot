package ffmpeg

import "time"

type RelayState string

const (
	StateStarting RelayState = "starting"
	StateRunning  RelayState = "running"
	StateStopping RelayState = "stopping"
	StateFailed   RelayState = "failed"
)

type StartRequest struct {
	TargetID string
	Label    string
	Mode     string
	Binary   string
	LogLevel string
	Input    string
	Output   string
	Args     []string
}

type RelayStatus struct {
	TargetID  string     `json:"target_id"`
	Label     string     `json:"label"`
	Mode      string     `json:"mode"`
	State     RelayState `json:"state"`
	PID       int        `json:"pid"`
	StartedAt time.Time  `json:"started_at"`
	LastError string     `json:"last_error,omitempty"`
	Logs      []string   `json:"logs,omitempty"`
}

func BuildArgs(req StartRequest) ([]string, error) {
	if len(req.Args) > 0 {
		return req.Args, nil
	}
	// Fallback/Default args
	return []string{"-i", req.Input, "-f", "flv", req.Output}, nil
}
