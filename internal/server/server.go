package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/squoyster/golivepilot/internal/config"
)

type Runtime interface {
	Status() map[string]any
	StartPreview(ctx context.Context) error
	StartGoLive(ctx context.Context) error
	StopAll()
}

type Server struct {
	cfg         *config.Config
	runtime     Runtime
	operatorPSK string
	templates   *template.Template
	version     string
}

func NewServer(cfg *config.Config, runtime Runtime, operatorPSK string, version string) *Server {
	return &Server{
		cfg:         cfg,
		runtime:     runtime,
		operatorPSK: operatorPSK,
		templates:   parseTemplates(),
		version:     version,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("POST /api/preview", s.withAuth(s.handlePreview))
	mux.HandleFunc("POST /api/go-live", s.withAuth(s.handleGoLive))
	mux.HandleFunc("POST /api/stop", s.withAuth(s.handleStop))

	return requestLog(mux)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == "" || s.cfg.Auth.Mode == "none" {
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.operatorPSK
		if s.operatorPSK == "" || auth != expected {
			slog.Warn("unauthorized request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	previewURL := s.cfg.UI.PreviewHLSURL

	// Legacy/Automatic resolution if not explicitly configured
	if previewURL == "" {
		if s.cfg.MediaEngine.Type == "mediamtx" {
			base := strings.TrimSuffix(s.cfg.MediaEngine.MediaMTX.HLSBaseURL, "/")
			previewURL = fmt.Sprintf("%s/live/preview/index.m3u8", base)
		} else if len(s.cfg.Ingests) > 0 {
			ing := s.cfg.Ingests[0]
			if ing.Preview.URL != "" {
				previewURL = ing.Preview.URL
			}
		}
	}

	previewURLJSON, _ := json.Marshal(previewURL)

	data := map[string]any{
		"Title":          firstNonEmpty(s.cfg.UI.Title, s.cfg.App.Name, "GoLivePilot"),
		"Subtitle":       firstNonEmpty(s.cfg.UI.Subtitle, "Preview → Go Live → Stop"),
		"Version":        s.version,
		"PreviewURL":     previewURL,
		"PreviewURLJSON": template.JS(previewURLJSON),
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runtime.Status())
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("method", r.Method, "path", r.URL.Path)
	logger.Info("api call")
	if err := s.runtime.StartPreview(r.Context()); err != nil {
		// Log with context-specific details if possible, but handlePreview
		// doesn't know the target yet. The runtime should log with target_id.
		logger.Error("preview failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"action": "preview",
	})
}

func (s *Server) handleGoLive(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("method", r.Method, "path", r.URL.Path)
	logger.Info("api call")
	if err := s.runtime.StartGoLive(r.Context()); err != nil {
		logger.Error("go-live failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"action": "go-live",
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("method", "POST", "path", "/api/stop")
	logger.Info("api call")
	s.runtime.StopAll()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"action": "stop",
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseTemplates() *template.Template {
	const index = `
{{define "index"}}
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.Title}}</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body { font-family: system-ui, sans-serif; margin: 0; background: #111; color: #eee; }
    main { max-width: 900px; margin: 0 auto; padding: 24px; }
    h1 { margin-bottom: 0; }
    .sub { color: #aaa; margin-top: 4px; }
    .card { background: #1c1c1c; border: 1px solid #333; border-radius: 12px; padding: 16px; margin: 16px 0; }
    button { font-size: 18px; padding: 14px 18px; margin: 6px; border-radius: 10px; border: 0; cursor: pointer; }
    button.primary { background: #2d7ef7; color: white; }
    button.danger { background: #b42318; color: white; }
    pre { background: #050505; color: #ddd; padding: 12px; border-radius: 8px; overflow: auto; }
    .video-container { width: 100%; aspect-ratio: 16/9; background: black; border-radius: 8px; overflow: hidden; margin-top: 12px; position: relative; }
    video { width: 100%; height: 100%; object-fit: contain; }
    .video-overlay { position: absolute; top: 0; left: 0; padding: 8px; background: rgba(0,0,0,0.5); font-size: 12px; }
  </style>
  <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
</head>
<body>
<main>
  <h1>{{.Title}}</h1>
  <p class="sub">{{.Subtitle}}</p>

  <div class="card">
    <h2>Preview</h2>
    <div class="video-container">
      <video id="preview-video" controls autoplay muted playsinline></video>
      <div class="video-overlay" id="preview-status">Standby</div>
    </div>
  </div>

  <div class="card">
    <h2>Controls</h2>
    <button onclick="post('/api/preview')">Preview</button>
    <button class="primary" onclick="post('/api/go-live')">Go Live</button>
    <button class="danger" onclick="post('/api/stop')">Stop</button>
  </div>

  <div class="card">
    <h2>Status</h2>
    <pre id="status">loading...</pre>
  </div>

  <p class="sub">Version: {{.Version}}</p>
</main>

<script>
let token = sessionStorage.getItem("golivepilot_token") || "";
if (!token) {
  token = prompt("Operator token, if required:") || "";
  sessionStorage.setItem("golivepilot_token", token);
}

async function api(path, options = {}) {
  const headers = {"Content-Type": "application/json"};
  if (token) headers["Authorization"] = "Bearer " + token;

  const res = await fetch(path, {...options, headers});
  const text = await res.text();

  try { return JSON.parse(text); }
  catch { return {status: res.status, text}; }
}

async function post(path) {
  const result = await api(path, {method: "POST"});
  await refresh();
  console.log(result);
  if (path === '/api/preview' || path === '/api/go-live') {
    reloadPlayer();
  }
}

async function refresh() {
  const result = await api("/api/status");
  document.getElementById("status").textContent = JSON.stringify(result, null, 2);
}

setInterval(refresh, 2000);
refresh();

const video = document.getElementById('preview-video');
const videoSrc = {{.PreviewURLJSON}};
const previewStatus = document.getElementById('preview-status');
let hls = null;

function reloadPlayer() {
  console.log("Reloading player...");
  if (hls) {
    hls.destroy();
    hls = null;
  }
  video.pause();
  video.removeAttribute('src');
  video.load();
  initPlayer();
}

function initPlayer() {
  if (!videoSrc) {
    previewStatus.textContent = "No preview HLS URL configured";
    return;
  }

  if (video.canPlayType('application/vnd.apple.mpegurl')) {
    // Native HLS support (Safari)
    video.src = videoSrc;
    video.onloadedmetadata = function() {
      video.play().catch(e => console.warn("Autoplay blocked", e));
      previewStatus.textContent = "Live Preview (Native)";
    };
    
    video.onplaying = function() {
      previewStatus.textContent = "Live Preview (Native)";
    };

    // Simple retry for native video
    video.onerror = function() {
      console.log("Native video error, retrying in 2s...");
      previewStatus.textContent = "Connecting...";
      setTimeout(() => {
        video.src = videoSrc + "?t=" + Date.now();
      }, 2000);
    };
  } else if (Hls.isSupported()) {
    hls = new Hls({
      manifestLoadingRetryDelay: 1000,
      manifestLoadingMaxRetry: Infinity,
      levelLoadingRetryDelay: 1000,
      levelLoadingMaxRetry: Infinity,
      fragLoadingRetryDelay: 1000,
      fragLoadingMaxRetry: Infinity,
      enableWorker: true,
      lowLatencyMode: true,
      backBufferLength: 60
    });
    hls.loadSource(videoSrc);
    hls.attachMedia(video);
    hls.on(Hls.Events.MANIFEST_PARSED, function() {
      video.play().catch(e => console.warn("Autoplay blocked", e));
      previewStatus.textContent = "Live Preview";
    });
    video.onplaying = function() {
      previewStatus.textContent = "Live Preview";
    };
    hls.on(Hls.Events.ERROR, function(event, data) {
      if (data.fatal) {
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            console.log("fatal network error encountered, try to recover");
            hls.startLoad();
            previewStatus.textContent = "Connecting...";
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            console.log("fatal media error encountered, try to recover");
            hls.recoverMediaError();
            break;
          default:
            console.error("Unrecoverable HLS error", data);
            previewStatus.textContent = "Error: " + data.details;
            // Retry entire player after a delay
            setTimeout(reloadPlayer, 5000);
            break;
        }
      }
    });
  } else {
    previewStatus.textContent = "HLS not supported in this browser";
  }
}

initPlayer();
</script>
</body>
</html>
{{end}}
`
	return template.Must(template.New("root").Parse(index))
}
