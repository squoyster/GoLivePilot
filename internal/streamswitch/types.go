package streamswitch

import (
	"time"
)

// SourceID identifies which input source is selected.
type SourceID string

const (
	SourceUnknown SourceID = ""
	SourceSlate   SourceID = "__slate_source__"
	SourceCamera  SourceID = "__camera_source__"
)

// InputConfig defines a source that feeds the StreamSwitch.
type InputConfig struct {
	ID   SourceID
	Args []string // ffmpeg args before pipe:1 (excluding "ffmpeg" binary)
}

// OutputConfig defines a target that receives the selected stream.
type OutputConfig struct {
	TargetID string
	URL      string // Full RTMPS/SRT/etc URL
	// If true, re-encode with the given args instead of -c copy.
	// Empty means copy (passthrough).
	EncodeArgs []string
}

// Config configures a StreamSwitch instance.
type Config struct {
	Inputs  []InputConfig
	Outputs []OutputConfig
	// Maximum time to wait for input to become available on start.
	StartTimeout time.Duration
}