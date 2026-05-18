package app

import (
	"context"

	"github.com/squoyster/golivepilot/internal/ffmpeg"
)

type RelaySupervisor interface {
	Start(ctx context.Context, req ffmpeg.StartRequest) error
	Stop(ctx context.Context, targetID string) error
	Switch(ctx context.Context, req ffmpeg.StartRequest) error
	StopAll(ctx context.Context)
	Status() map[string]ffmpeg.RelayStatus
}
