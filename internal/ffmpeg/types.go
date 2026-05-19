package ffmpeg

import (
	"strings"
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

	// Input arguments (can include multiple -i flags)
	// Example: -f lavfi -i anullsrc -re -loop 1 -i slate.png
	args = append(args, req.InputArgs...)

	// If main Input is set and not already in InputArgs (simple case), add it
	// Note: In complex cases like dual-input, the caller might put all -i in InputArgs.
	mainInputPresent := false
	for _, a := range req.InputArgs {
		if a == "-i" {
			// This is a bit heuristic, but if there's already an -i we assume
			// the caller might have handled it. However, StartRequest.Input
			// is usually the primary media.
		}
	}

	// If the primary input isn't in InputArgs, append it.
	// Check if req.Input is actually used as an argument to an existing -i
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

	// Fallback encoding if nothing specified in OutputArgs
	hasCodec := false
	for _, a := range req.OutputArgs {
		if strings.HasPrefix(a, "-c") || a == "-codec" {
			hasCodec = true
			break
		}
	}
	if !hasCodec {
		args = append(args, "-c", "copy")
	}

	// Set loglevel
	args = append(args, "-loglevel", req.LogLevel)

	// Always output as FLV to the RTMPS URL
	args = append(args, "-f", "flv", req.Output)

	return args, nil
}
