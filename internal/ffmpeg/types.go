package ffmpeg

import (
	"time"
)

type RelayState string

const (
	StateStarting RelayState = "starting"
	StateRunning  RelayState = "running"
	StateStopping RelayState = "stopping"
	StateFailed   RelayState = "failed"
)

type StartRequest struct {
	TargetID   string
	Label      string
	Mode       string
	Binary     string
	LogLevel   string
	Input      string
	Output     string
	InputArgs  []string
	OutputArgs []string
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
	var args []string

	// Hide banner for cleaner logs
	args = append(args, "-hide_banner")

	// Input arguments (can include multiple -i flags)
	args = append(args, req.InputArgs...)

	// If main Input is set and not already in InputArgs, add it
	mainInputPresent := false
	for i, a := range req.InputArgs {
		if a == "-i" && i+1 < len(req.InputArgs) && req.InputArgs[i+1] == req.Input {
			mainInputPresent = true
			break
		}
	}

	if !mainInputPresent && req.Input != "" {
		args = append(args, "-i", req.Input)
	}

	// Profiles and other output-side arguments
	args = append(args, req.OutputArgs...)

	// Set loglevel
	args = append(args, "-loglevel", req.LogLevel)

	// Always output as FLV to the RTMPS URL
	args = append(args, "-f", "flv", req.Output)

	return args, nil
}
