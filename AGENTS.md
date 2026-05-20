# GoLivePilot - Agent Instructions

## Project Overview
- **Type**: Go application - live broadcast control appliance for streaming to Facebook/YouTube/RTMPS
- **Module**: `github.com/squoyster/golivepilot`
- **Go version**: 1.26.3
- **Current version**: 0.1.59
- **Dependencies**: Only `gopkg.in/yaml.v3`

## Architecture
Three-layer design:
1. **Control plane**: Go service (`cmd/golivepilot/main.go`)
2. **Media plane**: FFmpeg workers (supervised by Go, external binary)
3. **Ingest/router**: MediaMTX (default, replaceable via `SourceEngine` abstraction)

The Go process does NOT implement RTMP/RTMPS/WebRTC/encoding directly. It supervises FFmpeg processes.

## Key Directories
```
cmd/golivepilot/main.go      # Entry point: flag parsing, config load, HTTP server
internal/app/                 # Runtime state, pipeline orchestration, relay supervision
internal/config/              # YAML config loading and validation
internal/ffmpeg/              # FFmpeg process supervisor
internal/mediamtx/            # MediaMTX API client
internal/server/              # HTTP handlers and routing
configs/                      # YAML configs (golivepilot.yml, mediamtx.yml)
web/                          # Static/templates (currently empty)
```

Note: `internal/state/`, `internal/platform/`, `internal/web/` directories exist but are empty (planned for future).

## Developer Commands
```bash
make run          # Run locally: go run ./cmd/golivepilot
make test         # Run all tests: go test ./...
make build        # Build binary: go build -o bin/golivepilot ./cmd/golivepilot
make docker-build # Build Docker image
make docker-up    # docker compose up -d --build
make docker-down  # docker compose down
make logs         # docker compose logs -f
```

### Running locally on macOS
- Run Go natively: `go run ./cmd/golivepilot`
- Run MediaMTX in Docker separately
- Config defaults to `/config/golivepilot.yml` (override with `--config` or `GOLIVEPILOT_CONFIG`)
- Default listen address: `:3000`

## Config & Secrets
- **Config**: Single YAML file (`configs/golivepilot.yml`)
- **Secrets**: Environment variables only (never in browser localStorage)
- Key env vars: `GOLIVEPILOT_OPERATOR_PSK`, `FB_RTMPS_KEY`, `YT_RTMPS_URL`, etc.
- Run with `--env-help` to see all relevant env vars

## Pipeline Architecture
The app uses a state machine with states: `standby` → `preview` → `live` → `ended`
- Pipeline nodes are FFmpeg workers started/stopped per state
- Background restarter (default 5s interval) monitors and restarts failed relays
- `__internal_source__` → `__program_source__` → platform relays (Facebook, YouTube, custom RTMPS)

## FFmpeg Supervisor
- FFmpeg is treated as an external worker process
- One active publisher per target endpoint (single-writer policy)
- Per-target reconnect with configurable retry policy
- Modes: `relay`, `diag`, `slate`
- Logs captured from stderr, retained per relay

## Testing
- Tests exist in: `internal/ffmpeg/`, `internal/config/`, `internal/app/`, `internal/server/`, `internal/mediamtx/`
- Run: `go test ./...` or `make test`
- No special test fixtures or services required for unit tests

## Important Constraints
- Do NOT open two concurrent RTMPS connections to same stream key
- Do NOT recreate/delete Facebook/YouTube broadcast while viewers waiting
- Secrets stay server-side (PSK, tokens, stream keys)
- Browser localStorage only for UI preferences
- v0.1 does NOT require seamless slate-to-camera switching

## Docker Deployment
- Default: Docker Compose with golivepilot + MediaMTX services
- Dockerfile is currently empty (needs implementation)
- docker-compose.yml is currently empty (needs implementation)
- Image strategy: multi-stage build, Go binary + FFmpeg in slim Debian container
