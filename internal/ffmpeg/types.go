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
	var args []string

	// Input arguments (placed before -i)
	var inputArgs []string
	// Output arguments (placed after -i <input> and before the final output URL)
	var outputArgs []string

	// Heuristically separate input and output arguments from req.Args.
	// This is tricky because req.Args is currently a mix.
	// Let's assume req.Args starts with input options (-re, -loop, etc.)
	// and continues with output options.

	i := 0
	for i < len(req.Args) {
		arg := req.Args[i]
		if arg == "-re" || arg == "-loop" || arg == "-stream_loop" {
			inputArgs = append(inputArgs, arg)
			if (arg == "-loop" || arg == "-stream_loop") && i+1 < len(req.Args) {
				inputArgs = append(inputArgs, req.Args[i+1])
				i += 2
			} else {
				i++
			}
		} else {
			// Once we hit something that isn't a known input flag, assume the rest are output flags.
			outputArgs = append(outputArgs, req.Args[i:]...)
			break
		}
	}

	args = append(args, inputArgs...)
	args = append(args, "-i", req.Input)
	args = append(args, outputArgs...)

	// Fallback encoding if nothing specified in outputArgs
	hasCodec := false
	for _, a := range outputArgs {
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
