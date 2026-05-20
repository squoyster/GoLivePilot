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
	StateStopped  RelayState = "stopped"
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

	// Set loglevel early
	args = append(args, "-loglevel", req.LogLevel)

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

	// Always output as FLV to the RTMPS URL.
	// We check if -f flv is already present in OutputArgs to avoid duplication.
	flvPresent := false
	for i, a := range req.OutputArgs {
		if a == "-f" && i+1 < len(req.OutputArgs) && req.OutputArgs[i+1] == "flv" {
			flvPresent = true
			break
		}
	}

	if !flvPresent {
		args = append(args, "-f", "flv")
	}

	if req.Output != "" {
		args = append(args, req.Output)
	}

	return args, nil
}
