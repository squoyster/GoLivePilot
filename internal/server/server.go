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
	HardStop()
	DiagFacebook(ctx context.Context, targetID string) ([]string, error)
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
	mux.HandleFunc("GET /assets/", s.handleAssets)
	mux.HandleFunc("GET /api/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("POST /api/preview", s.withAuth(s.handlePreview))
	mux.HandleFunc("POST /api/go-live", s.withAuth(s.handleGoLive))
	mux.HandleFunc("POST /api/stop", s.withAuth(s.handleStop))
	mux.HandleFunc("POST /api/diag/facebook", s.withAuth(s.handleDiagFacebook))

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
	if r.URL.Query().Get("hard") == "true" {
		s.runtime.HardStop()
	} else {
		s.runtime.StopAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"action": "stop",
	})
}

func (s *Server) handleDiagFacebook(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("method", "POST", "path", "/api/diag/facebook")
	logger.Info("api call")
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		targetID = "facebook-main"
	}

	logs, err := s.runtime.DiagFacebook(r.Context(), targetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"logs":  logs,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"logs": logs,
	})
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "assets/"+path)
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
    button { font-size: 18px; padding: 14px 18px; margin: 6px; border-radius: 10px; border: 0; cursor: pointer; transition: background 0.2s; }
    button.preview { background: #2d7ef7; color: white; }
    button.live { background: #059669; color: white; }
    button.stop { background: #b42318; color: white; }
    button:active { filter: brightness(0.8); }

    .controls-grid { display: flex; flex-wrap: wrap; gap: 12px; }
    .controls-grid button { flex: 1; min-width: 200px; margin: 0; }
    .controls-grid button.reset { background: #444; color: #ccc; }

    @media (max-width: 600px) {
      .controls-grid { flex-direction: column; }
      .controls-grid button { width: 100%; height: 60px; font-weight: bold; }
      /* Add extra spacing for the stop button on mobile to prevent accidents */
      .controls-grid button.stop { margin-top: 12px; }
    }
    pre { background: #050505; color: #ddd; padding: 12px; border-radius: 8px; overflow: auto; }
    .video-container { width: 100%; aspect-ratio: 16/9; background: black; border-radius: 8px; overflow: hidden; margin-top: 12px; position: relative; }
    video { width: 100%; height: 100%; object-fit: contain; }
    .video-placeholder { position: absolute; top: 0; left: 0; width: 100%; height: 100%; background: #000 no-repeat center center; background-size: contain; display: none; }
    .video-placeholder.standing { background-image: url('/assets/standing-by.png'); }
    .video-placeholder.starting { background-image: url('/assets/starting-soon.png'); }
    .video-placeholder.ended { background-image: url('/assets/stream-ended.png'); }
    .video-overlay { position: absolute; top: 0; left: 0; padding: 4px 8px; background: rgba(0,0,0,0.7); font-size: 14px; font-weight: bold; border-bottom-right-radius: 8px; }
    .video-overlay.live { color: #ff4d4d; }

    /* Status Stepper */
    .status-stepper { display: flex; justify-content: space-between; margin-bottom: 24px; position: relative; }
    .status-stepper::before { content: ''; position: absolute; top: 18px; left: 0; right: 0; height: 2px; background: #333; z-index: 1; }
    .step { z-index: 2; background: #111; padding: 0 10px; display: flex; flex-direction: column; align-items: center; flex: 1; }
    .step-circle { width: 36px; height: 36px; border-radius: 50%; background: #333; display: flex; align-items: center; justify-content: center; font-weight: bold; margin-bottom: 8px; transition: all 0.3s; border: 2px solid #111; }
    .step-label { font-size: 12px; color: #888; transition: all 0.3s; }
    .step.active .step-circle { background: #2d7ef7; color: white; box-shadow: 0 0 15px rgba(45, 126, 247, 0.5); }
    .step.active .step-label { color: #fff; font-weight: bold; }
    .step.completed .step-circle { background: #059669; color: white; }
    .step.completed .step-label { color: #aaa; }
    .step.live .step-circle { background: #b42318; box-shadow: 0 0 15px rgba(180, 35, 24, 0.5); }
  </style>
  <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
</head>
<body>
<main>
  <h1>{{.Title}}</h1>
  <p class="sub">{{.Subtitle}}</p>

  <div class="card">
    <div class="status-stepper" id="stepper">
      <div class="step" id="step-standby">
        <div class="step-circle">1</div>
        <div class="step-label">Standby</div>
      </div>
      <div class="step" id="step-slate">
        <div class="step-circle">2</div>
        <div class="step-label">Preview</div>
      </div>
      <div class="step" id="step-camera">
        <div class="step-circle">3</div>
        <div class="step-label">Go Live</div>
      </div>
      <div class="step" id="step-ended">
        <div class="step-circle">4</div>
        <div class="step-label">Stream Ended</div>
      </div>
    </div>
  </div>

  <div class="card">
    <h2 id="viewer-header">Live Stream Viewer</h2>
    <div class="video-container">
      <video id="preview-video" controls autoplay muted playsinline></video>
      <div id="video-placeholder" class="video-placeholder"></div>
      <div class="video-overlay" id="preview-status">Standby</div>
    </div>
  </div>

  <div class="card">
    <h2>Controls</h2>
    <div class="controls-grid">
      <button class="preview" onclick="post('/api/preview')">Preview</button>
      <button class="live" onclick="post('/api/go-live')">Go Live</button>
      <button class="stop" id="btn-stop" onclick="post('/api/stop')">Stop Stream</button>
      <button class="reset" id="btn-reset" onclick="post('/api/stop?hard=true')">Reset Everything</button>
      <button class="diag" id="btn-diag-fb" onclick="diagFacebook()" style="background-color: #6366f1; color: white;">Test Facebook</button>
    </div>
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
    // Give backend a moment to start FFmpeg and MediaMTX to update
    setTimeout(reloadPlayer, 1000);
  }
}

const video = document.getElementById('preview-video');
const videoSrc = {{.PreviewURLJSON}};
const previewStatus = document.getElementById('preview-status');
let hls = null;

// Status Management
class StatusManager {
  constructor() {
    this.status = {
      source_mode: 'unknown',
      relays: {}
    };
    this.listeners = [];
  }

  subscribe(callback) {
    this.listeners.push(callback);
    // Call immediately with current state
    callback(this.status);
  }

  update(newStatus) {
    if (JSON.stringify(this.status) === JSON.stringify(newStatus)) return;
    const oldStatus = this.status;
    this.status = newStatus;
    this.listeners.forEach(cb => cb(newStatus, oldStatus));
  }
}

const statusManager = new StatusManager();

// Register listeners for DOM updates
statusManager.subscribe((status) => {
  // Update the raw status pre
  document.getElementById("status").textContent = JSON.stringify(status, null, 2);

  if (!status.source_mode) return;

  // Update Labels and Styles
  let label = "Standby";
  let headerLabel = "Live Stream Viewer";
  
  switch (status.source_mode) {
    case "standby": 
      label = "Standby"; 
      headerLabel = "Live Stream Viewer: Standby";
      break;
    case "slate": 
      label = "Preview (Slate)"; 
      headerLabel = "Live Stream Viewer: Previewing";
      break;
    case "camera": 
      label = "LIVE"; 
      headerLabel = "Live Stream Viewer: BROADCASTING";
      break;
    case "ended": 
      label = "Stream Ended"; 
      headerLabel = "Live Stream Viewer: Ended";
      break;
    case "stopped":
      label = "Stopped";
      headerLabel = "Live Stream Viewer: Stopped";
      break;
  }

  if (previewStatus) {
    previewStatus.textContent = label;
    if (status.source_mode === "camera") {
      previewStatus.classList.add("live");
    } else {
      previewStatus.classList.remove("live");
    }
  }

  const viewerHeader = document.getElementById("viewer-header");
  if (viewerHeader) viewerHeader.textContent = headerLabel;

  // Update Buttons
  const btnStop = document.getElementById("btn-stop");
  const btnReset = document.getElementById("btn-reset");
  if (btnStop) {
    if (status.source_mode === "ended" || status.source_mode === "stopped") {
      btnStop.disabled = true;
      btnStop.style.opacity = "0.5";
    } else {
      btnStop.disabled = false;
      btnStop.style.opacity = "1";
    }
  }

  // Update Stepper
  const steps = ['standby', 'slate', 'camera', 'ended'];
  steps.forEach(function(mode) {
    var el = document.getElementById('step-' + mode);
    if (!el) return;
    el.classList.remove('active', 'completed', 'live');
    
    if (status.source_mode === mode) {
      el.classList.add('active');
      if (mode === 'camera') el.classList.add('live');
    } else {
      // Mark previous steps as completed
      const currentIndex = steps.indexOf(status.source_mode);
      const stepIndex = steps.indexOf(mode);
      if (currentIndex !== -1 && stepIndex < currentIndex) {
        el.classList.add('completed');
      }
    }
  });

  // Handle Player vs Placeholder
  const placeholder = document.getElementById("video-placeholder");
  if (placeholder && video) {
    if (status.source_mode === "ended" || status.source_mode === "standby" || status.source_mode === "stopped") {
      stopPlayer();
      placeholder.style.display = "block";
      video.style.display = "none";
      
      // Fix: Explicitly manage classes to ensure correct image
      if (status.source_mode === "ended") {
        placeholder.classList.add("ended");
        placeholder.classList.remove("starting", "standing");
      } else if (status.source_mode === "standby" || status.source_mode === "stopped") {
        placeholder.classList.add("standing");
        placeholder.classList.remove("starting", "ended");
      } else {
        placeholder.classList.add("starting");
        placeholder.classList.remove("standing", "ended");
      }
    } else {
      placeholder.style.display = "none";
      video.style.display = "block";
    }
  }
});

async function refresh() {
  const result = await api("/api/status");
  statusManager.update(result);
}

async function diagFacebook() {
  const btn = document.getElementById("btn-diag-fb");
  const originalText = btn.textContent;
  btn.textContent = "Testing...";
  btn.disabled = true;
  
  try {
    const res = await api("/api/diag/facebook", "POST");
    if (res.ok) {
      alert("Facebook Diagnostic PASSED!\n\nLogs:\n" + (res.logs || []).join("\n"));
    } else {
      alert("Facebook Diagnostic FAILED!\n\nError: " + res.error + "\n\nLogs:\n" + (res.logs || []).join("\n"));
    }
  } catch (e) {
    alert("Request failed: " + e);
  } finally {
    btn.textContent = originalText;
    btn.disabled = false;
  }
}

setInterval(refresh, 2000);
refresh();

function stopPlayer() {
  if (hls) {
    hls.destroy();
    hls = null;
  }
  if (video) {
    video.pause();
    video.removeAttribute('src');
    video.load();
  }
}

function reloadPlayer() {
  console.log("Reloading player...");
  stopPlayer();
  initPlayer();
}

function initPlayer() {
  if (!video || !videoSrc) {
    if (previewStatus) previewStatus.textContent = "No preview HLS URL configured";
    return;
  }

  if (video.canPlayType('application/vnd.apple.mpegurl')) {
    // Native HLS support (Safari)
    video.src = videoSrc;
    video.onloadedmetadata = function() {
      video.play().catch(e => console.warn("Autoplay blocked", e));
    };
    
    video.onplaying = function() {
      // Status updated by refresh()
    };

    // Simple retry for native video
    video.onerror = function() {
      console.log("Native video error, retrying in 2s...");
      // previewStatus.textContent = "Connecting..."; // Don't override the backend status
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
    });
    video.onplaying = function() {
      // Status updated by refresh()
    };
    hls.on(Hls.Events.ERROR, function(event, data) {
      if (data.fatal) {
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            console.log("fatal network error encountered, try to recover");
            hls.startLoad();
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            console.log("fatal media error encountered, try to recover");
            hls.recoverMediaError();
            break;
          default:
            console.error("Unrecoverable HLS error", data);
            if (previewStatus) previewStatus.textContent = "Error: " + data.details;
            // Retry entire player after a delay
            setTimeout(reloadPlayer, 5000);
            break;
        }
      }
    });
  } else {
    if (previewStatus) previewStatus.textContent = "HLS not supported in this browser";
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
