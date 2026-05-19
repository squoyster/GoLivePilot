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

	// If we have profile-like args, we need to be careful about where -i goes.
	// Usually -re -loop 1 goes BEFORE -i.
	// Let's assume req.Args contains flags that might need to be before OR after -i.
	// But in our current StartPreview, we put -re -loop before profile args.

	// A better way: if req.Args exists, it's the middle part.
	// We'll put -i req.Input at the start if it's not already in req.Args (not likely here).

	// Actually, let's keep it simple and fix BuildArgs to NOT be too smart but be correct.
	// If req.Args is provided, we assume it contains everything EXCEPT the -i <input> and the final output.
	// Wait, some flags MUST go before -i (like -re, -loop).
	// Let's change the strategy: StartPreview should provide the FULL argument list if it wants control.

	args = append(args, req.Args...)

	// Insert -i if it's missing (heuristically)
	hasInput := false
	for _, a := range args {
		if a == "-i" {
			hasInput = true
			break
		}
	}
	if !hasInput {
		// Try to find a good spot for -i. Usually after -re etc.
		// For now, let's just prepend it if no flags are there, or append it after the first few flags.
		// Simplest: if we have req.Input, and no -i in args, we must add it.
		// We'll put it at the beginning, but AFTER any -re or -loop.

		insertIdx := 0
		for i, a := range args {
			if strings.HasPrefix(a, "-") && (a == "-re" || a == "-loop" || a == "-stream_loop") {
				// skip these and their values if any
				insertIdx = i + 1
				if a != "-re" && i+1 < len(args) {
					insertIdx = i + 2
				}
			} else {
				break
			}
		}

		if insertIdx > len(args) {
			insertIdx = len(args)
		}

		newArgs := make([]string, 0, len(args)+2)
		newArgs = append(newArgs, args[:insertIdx]...)
		newArgs = append(newArgs, "-i", req.Input)
		newArgs = append(newArgs, args[insertIdx:]...)
		args = newArgs
	}

	// Fallback encoding if nothing specified
	hasCodec := false
	for _, a := range args {
		if strings.HasPrefix(a, "-c") || a == "-codec" {
			hasCodec = true
			break
		}
	}
	if !hasCodec {
		args = append(args, "-c", "copy")
	}

	// Always output as FLV to the RTMPS URL
	args = append(args, "-f", "flv", req.Output)

	return args, nil
}
