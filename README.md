# GoLivePilot

GoLivePilot is a lightweight, self-hosted live broadcast control appliance for sending one simple operator-controlled stream to platforms such as Facebook Live and YouTube Live.

The intended user experience is deliberately simple:

```text
Open web app → allow camera/mic → Preview → Go Live → Stop
```

The system is designed for a VPS-hosted deployment with Docker, a small Go control service, FFmpeg for media handling, and platform adapters for Facebook, YouTube, and future targets.

---

## Goals

GoLivePilot is intended to provide:

- A simple browser-based control UI for nontechnical operators.
- Configurable ingestion endpoints.
- Configurable target endpoints.
- Facebook and YouTube adapters supporting preview/go-live/end workflows.
- A static image or looped “stream starting soon” slate during preview.
- A live camera preview in the browser.
- FFmpeg-based media handling.
- Persistent runtime state.
- Dockerized deployment.
- A mostly single-binary Go control application.
- A plugin-style architecture for additional platforms.

---

## Non-goals for v0.1

The first version is intentionally limited. It should not attempt to be OBS, Restream, or a full broadcast production suite.

Out of scope for v0.1:

- Complex scene composition.
- Multi-camera switching.
- Lower-thirds/graphics overlays.
- Multi-user roles.
- Full OAuth setup wizard.
- Dynamic binary plugins.
- Perfect seamless slate/camera switching.
- Native iOS/Android apps.
- Direct browser RTMPS publishing.

---

## Core Architectural Decision

GoLivePilot separates the system into three layers:

```text
Control plane:  GoLivePilot Go service
Media plane:    FFmpeg workers
Ingest/router:  MediaMTX by default, replaceable later
```

### GoLivePilot Go Service

Responsible for:

- Serving the web UI.
- Loading configuration.
- Resolving secrets from environment variables or Docker secrets.
- Managing runtime state.
- Starting/stopping FFmpeg workers.
- Supervising FFmpeg processes.
- Retrying failed target connections where appropriate.
- Calling Facebook and YouTube APIs.
- Exposing target state, source state, and logs.

The Go process does **not** implement RTMP, RTMPS, WebRTC, or media encoding directly.

### FFmpeg

Responsible for:

- Generating the preview slate feed.
- Reading the live source feed.
- Encoding/remuxing/transcoding as needed.
- Opening outbound RTMPS connections to target platforms.
- Maintaining the media connection to Facebook, YouTube, or custom RTMPS targets.

FFmpeg is treated as an external worker process supervised by Go.

### MediaMTX

MediaMTX is the default ingest/router layer.

It is used for:

- Browser WebRTC ingest.
- Future RTMP/RTMPS/SRT/OBS/Larix ingest support.
- Stable internal stream URLs for FFmpeg.
- HLS or WebRTC preview endpoints.
- Source online/offline status.

MediaMTX is not responsible for Facebook/YouTube lifecycle management.

The architecture should keep MediaMTX replaceable through an internal `SourceEngine` abstraction.

---

## Why Keep MediaMTX?

The key design question is whether GoLivePilot should remove MediaMTX and let FFmpeg handle everything.

The decision for v0.1 is: **keep MediaMTX as the default source engine.**

Reasons:

- FFmpeg is excellent for media processing but awkward as an always-on ingest server.
- MediaMTX cleanly decouples the source from target outputs.
- Target relays can reconnect independently without disturbing the source.
- One source can feed multiple FFmpeg target workers.
- MediaMTX provides HLS/WebRTC preview with low implementation effort.
- Future input support for OBS, Larix, RTMP, RTMPS, SRT, or WebRTC is easier.

The target lifecycle problem remains in GoLivePilot, but MediaMTX makes the media-source side simpler and more robust.

---

## High-Level Flow

### Preview Flow

```text
Operator opens GoLivePilot
→ browser requests camera/mic permission
→ local preview appears
→ operator clicks Preview
→ GoLivePilot creates/prepares platform broadcasts if configured
→ GoLivePilot starts FFmpeg slate workers
→ Facebook/YouTube receive “Stream is starting soon”
→ platform preview status is displayed in UI
```

### Go Live Flow

```text
Operator clicks Go Live
→ browser camera feed is confirmed online
→ GoLivePilot starts/switches target FFmpeg workers to camera feed
→ GoLivePilot waits for platform ingest to become active/stable
→ GoLivePilot calls Facebook/YouTube go-live transitions
→ UI enters LIVE state
```

### Stop Flow

```text
Operator clicks Stop
→ GoLivePilot calls Facebook end / YouTube complete
→ GoLivePilot stops FFmpeg workers
→ browser publishing is stopped
→ runtime state is persisted
```

---

## Switching Policy

v0.1 does **not** require perfectly seamless slate-to-camera switching.

However, it should avoid disrupting viewers waiting for the stream to start.

Important invariant:

```text
Do not recreate or delete the Facebook/YouTube broadcast object while viewers are waiting.
```

For v0.1, it is acceptable to briefly reconnect the encoder feed before the public go-live transition, provided the platform event/watch page remains stable.

Future versions may implement a continuous internal “program output” feed so that target RTMPS connections never drop during source switching.

---

## Target Connection Policy

Each target stream key is treated as a **single-writer resource**.

GoLivePilot should enforce:

```text
one target endpoint = one active FFmpeg publisher
```

It should not open two concurrent RTMPS connections to the same Facebook or YouTube stream key.

Bad:

```text
slate FFmpeg  → same FB key
camera FFmpeg → same FB key at the same time
```

Preferred:

```text
stop old target worker
confirm exit
start replacement worker
```

For future seamless switching, use a stable internal program feed rather than concurrent platform publishers.

---

## Target Reconnect Policy

Facebook, YouTube, and custom RTMPS targets should be independently reconnectable.

Recommended worker model:

```text
source feed → FFmpeg worker for Facebook
source feed → FFmpeg worker for YouTube
source feed → FFmpeg worker for custom target
```

Benefits:

- Facebook failure does not stop YouTube.
- YouTube failure does not stop Facebook.
- Each target has independent logs and state.
- Each target can implement its own retry policy.
- Platform adapters can classify retryable vs permanent failures.

Example target states:

```text
STOPPED
STARTING
PREVIEWING
LIVE
RECONNECTING
ENDING
ENDED
ERROR
```

Retry policy should be configurable per target.

---

## Plugin/Adapter Architecture

Facebook, YouTube, and custom RTMPS should be implemented as platform adapters behind a common interface.

Conceptual Go interface:

```go
type PlatformAdapter interface {
    Name() string

    PreparePreview(ctx context.Context, target TargetRuntime) error
    GetIngestTarget(ctx context.Context, target TargetRuntime) (*IngestTarget, error)

    PollPreviewReady(ctx context.Context, target TargetRuntime) (bool, error)
    GoLive(ctx context.Context, target TargetRuntime) error
    End(ctx context.Context, target TargetRuntime) error

    GetStatus(ctx context.Context, target TargetRuntime) (*PlatformStatus, error)
}
```

### Facebook Adapter

Responsibilities:

- Resolve RTMPS ingest URL from config or API-created LiveVideo.
- Support preview/preparation where applicable.
- Transition broadcast to live.
- End the live video.
- Poll platform status.

Typical action:

```text
POST /{live-video-id} status=LIVE_NOW
```

### YouTube Adapter

Responsibilities:

- Resolve RTMPS ingest URL from configured or API-created live stream.
- Poll stream health / active ingest state.
- Transition broadcast to testing/preview if needed.
- Transition broadcast to live.
- Complete the broadcast.
- Refresh OAuth tokens when configured.

### Custom RTMPS Adapter

Responsibilities:

- Provide configured RTMPS URL.
- No-op for preview/go-live/end unless explicitly configured.

This supports arbitrary targets such as Twitch, Rumble, Kick, or private RTMPS servers later.

---

## Source Engine Abstraction

Although MediaMTX is the default, the application should not hardcode it everywhere.

Conceptual interface:

```go
type SourceEngine interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    GetSourceURL(ingestID string) string
    GetPreviewURL(ingestID string) string
    GetStatus(ctx context.Context, ingestID string) (SourceStatus, error)
}
```

Initial implementation:

```text
mediamtx
```

Possible future implementations:

```text
ffmpeg-listen
websocket-chunks
native-webrtc
direct-rtmp
srt
```

---

## Configuration Model

GoLivePilot supports two configuration modes:

### Single file mode

```bash
golivepilot --config /path/to/golivepilot.yml
```

### Directory mode (recommended)

```bash
golivepilot --config-dir configs/local
```

All `*.yml` files in the directory are loaded in alphabetical order and merged. Later files override earlier ones.

### Config directory layout

```
configs/
  .gitignore            # ignores everything except example/
  example/              # checked-in templates (safe to commit)
    golivepilot.yml
    mediamtx.yml
  local/                # local development (gitignored)
    golivepilot.yml
    mediamtx.yml
  production/           # production deployment (gitignored)
    golivepilot.yml
    mediamtx.yml
```

Only `configs/example/` is tracked in git. All other directories are ignored to prevent accidental commits of secrets.

Create your own environment directory:

```bash
cp -r configs/example configs/local
# Edit configs/local/golivepilot.yml with your settings
golivepilot --config-dir configs/local
```

### Environment variables

Config values can be overridden with environment variables. Secrets should **only** be set via environment variables, never in YAML files.

```bash
# Override listen address
golivepilot --config-dir configs/local --listen :8080

# Set secrets via environment
export GOLIVEPILOT_OPERATOR_PSK=my-secret-key
export FB_RTMPS_KEY=FB-xxxxx-0-xxxxx
golivepilot --config-dir configs/local
```

### Example config

```yaml
app:
  name: "GoLivePilot"
  listen: ":3000"
  data_dir: "/data"

auth:
  mode: "psk_cookie"
  psk_env: "GOLIVEPILOT_OPERATOR_PSK"

media_engine:
  type: "mediamtx"
  mediamtx:
    api_url: "http://mediamtx:9997"
    internal_rtmp_base: "rtmp://mediamtx:1935"
    hls_base_url: "https://stream.example.com/hls"

slate:
  enabled: true
  type: "image"
  path: "assets/starting-soon.png"

ffmpeg:
  binary: "ffmpeg"
  log_level: "info"

targets:
  - id: "facebook-main"
    label: "Facebook"
    platform: "facebook"
    enabled: true
    profile_id: "x264-720p"
    rtmps_url_env: "rtmps://live-api-s.facebook.com:443/rtmp/"
    rtmps_key_env: "FB_RTMPS_KEY"
```

---

## Secrets Policy

Do not store platform tokens, RTMPS keys, stream keys, or publish passwords in browser localStorage.

Browser storage may hold UI preferences only.

Examples of acceptable browser-local values:

```text
selected tab
collapsed panels
dark mode
refresh interval
```

Secrets should remain server-side:

```text
Facebook page access token
YouTube OAuth tokens
RTMPS stream keys
MediaMTX publish passwords
operator PSK
```

For simple private deployments, a PSK login can be used.

Preferred login flow:

```text
/token-login?t=...
→ server validates token
→ server sets Secure HttpOnly SameSite cookie
→ redirect to /
```

The token should not remain in the URL after login.

---

## Runtime Persistence

GoLivePilot should persist runtime model and recent history.

v0.1 options:

- bbolt database.
- JSON event log.
- SQLite later if query/reporting needs grow.

Persistent state should include:

- Target runtime state.
- Last known platform state.
- FFmpeg worker state.
- Recent FFmpeg logs.
- Recent platform API errors.
- Last successful preview/go-live/end timestamps.
- Last known source state.

The application should be able to restart and recover enough state to show what was happening before the restart.

---

## Browser/Mobile Support

One web app should support mobile and desktop.

Official v0.1 support target:

```text
iOS Safari
Android Chrome
Desktop Chrome/Edge
```

The mobile UI should assume:

- Large buttons.
- Landscape mode preferred.
- Local preview shown with `playsinline` and `muted`.
- The page must remain open.
- The phone should not be locked during streaming.
- Network changes may interrupt capture.

The app should display simple operator guidance:

```text
Use landscape mode.
Keep this page open.
Do not lock the screen.
Use Wi-Fi or strong cellular.
Plug in power for long streams.
```

---

## Docker Deployment

The default deployment uses Docker Compose.

Recommended services:

```text
golivepilot   Go app + FFmpeg
mediamtx      source ingest/router/preview
caddy         optional HTTPS reverse proxy
```

Example shape:

```yaml
services:
  mediamtx:
    image: bluenviron/mediamtx:latest-ffmpeg
    container_name: mediamtx
    restart: unless-stopped
    ports:
      - "1936:1936"
      - "8888:8888"
      - "8889:8889"
    volumes:
      - ./configs/mediamtx.yml:/mediamtx.yml:ro
      - /etc/letsencrypt:/etc/letsencrypt:ro

  golivepilot:
    build: .
    image: golivepilot:dev
    container_name: golivepilot
    restart: unless-stopped
    depends_on:
      - mediamtx
    ports:
      - "3000:3000"
    env_file:
      - .env
    volumes:
      - ./configs/golivepilot.yml:/config/golivepilot.yml:ro
      - ./data:/data
    command:
      - "--config"
      - "/config/golivepilot.yml"
```

---

## Docker Image Strategy

The Go app should build to a single Linux binary, packaged with FFmpeg in a slim container image.

Recommended Dockerfile shape:

```dockerfile
FROM golang:1.23-bookworm AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/golivepilot ./cmd/golivepilot

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/golivepilot /usr/local/bin/golivepilot

EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/golivepilot"]
```

FFmpeg remains an external binary. Go supervises it rather than linking libav directly.

---

## Development Workflow

Recommended macOS development approach:

```text
Run Go natively on macOS.
Run MediaMTX in Docker.
Use Docker Compose for integration.
Build Linux image with multi-stage Dockerfile.
```

Typical commands:

```bash
# Create local config
cp -r configs/example configs/local

# Run with config directory
go run ./cmd/golivepilot --config-dir configs/local

# Run tests
go test ./...

# Docker
docker compose down; docker compose build --no-cache; docker compose up -d; docker compose logs -f
```

---

## Project Layout

```text
golivepilot/
  cmd/
    golivepilot/
      main.go         # Entry point, flag parsing, HTTP server start
  internal/
    app/
      runtime.go      # Runtime state management
    config/
      config.go       # Configuration structs
      load.go         # Config loading (single file or directory)
      validate.go     # Config validation
    ffmpeg/
      supervisor.go   # FFmpeg process management
    server/
      server.go       # HTTP handlers and routing
    platform/
      facebook/       # Future
      youtube/        # Future
  configs/
    .gitignore        # ignores everything except example/
    example/          # checked-in templates
      golivepilot.yml
      mediamtx.yml
    local/            # local development (gitignored)
    production/       # production deployment (gitignored)
  Dockerfile
  docker-compose.yml
  Makefile
  README.md
```

---

## v0.1 Milestones

### 1. Skeleton

- Go HTTP server.
- Static web UI.
- Config loading.
- Environment-secret resolution.
- PSK auth.

### 2. Runtime State

- Persistent state store.
- Target state model.
- Source state model.
- Basic event log.

### 3. FFmpeg Supervisor

- Start/stop target workers.
- Capture stderr logs.
- Track exit codes.
- Enforce one active worker per target.
- Basic reconnect policy.

### 4. Slate Preview

- Generate static image/silent audio slate.
- Push slate to configured RTMPS targets.
- Show platform preview status.

### 5. Browser Camera Ingest

- Browser camera/mic capture.
- Local preview.
- WebRTC publish to MediaMTX.
- Backend source status.

### 6. Facebook Adapter

- Manual LiveVideo ID + token config.
- Go live.
- End stream.
- Status polling.

### 7. YouTube Adapter

- Manual broadcast ID + RTMPS URL config.
- Transition to testing/preview where applicable.
- Transition to live.
- Complete broadcast.
- Token refresh support.

### 8. Docker Release

- Example compose.
- Example configs.
- README deployment guide.
- Basic troubleshooting.

---

## Design Summary

GoLivePilot should be a small, config-driven live broadcast appliance:

```text
Browser UI
  → GoLivePilot control service
  → FFmpeg workers
  → Facebook / YouTube / custom RTMPS

Browser camera
  → MediaMTX WebRTC ingest
  → FFmpeg target workers
```

Key principles:

- Keep the user workflow simple.
- Keep credentials server-side.
- Use FFmpeg for video work.
- Use adapters for platform-specific lifecycle control.
- Keep source ingest abstracted.
- Use one active publisher per target endpoint.
- Make reconnect and error handling per-target.
- Keep v0.1 intentionally unsophisticated.

