package app

import (
	"context"
	"time"

	"github.com/squoyster/golivepilot/internal/mediamtx"
)

// MediaMTXClient defines the subset of mediamtx.Client used by Runtime and Pipeline.
type MediaMTXClient interface {
	GetPath(ctx context.Context, path string) (*mediamtx.PathInfo, error)
	WaitUntilHealthy(ctx context.Context, timeout time.Duration) error
	WaitForPathReady(ctx context.Context, path string, timeout time.Duration) (*mediamtx.PathInfo, error)
}
