package server

import (
	"context"
	"encoding/json"
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
	data := map[string]any{
		"Title":    firstNonEmpty(s.cfg.UI.Title, s.cfg.App.Name, "GoLivePilot"),
		"Subtitle": firstNonEmpty(s.cfg.UI.Subtitle, "Preview → Go Live → Stop"),
		"Version":  s.version,
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
	logger := slog.With("method", "POST", "path", "/api/go-live")
	logger.Info("api call")
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
  </style>
</head>
<body>
<main>
  <h1>{{.Title}}</h1>
  <p class="sub">{{.Subtitle}}</p>

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
}

async function refresh() {
  const result = await api("/api/status");
  document.getElementById("status").textContent = JSON.stringify(result, null, 2);
}

setInterval(refresh, 2000);
refresh();
</script>
</body>
</html>
{{end}}
`
	return template.Must(template.New("root").Parse(index))
}
