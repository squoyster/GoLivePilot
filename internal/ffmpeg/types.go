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

	args = append(args, req.InputArgs...)
	args = append(args, "-i", req.Input)
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
