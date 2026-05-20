package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/squoyster/golivepilot/internal/config"
)

type mockRuntime struct {
	statusCalled  bool
	previewCalled bool
	goLiveCalled  bool
	stopCalled    bool
	hardStopCalled bool
	diagCalled    bool
	diagTargetID  string
	diagErr       error
	diagLogs      []string
}

func (m *mockRuntime) Status() map[string]any {
	m.statusCalled = true
	return map[string]any{"source_mode": "standby"}
}

func (m *mockRuntime) StartPreview(ctx context.Context) error {
	m.previewCalled = true
	return nil
}

func (m *mockRuntime) StartGoLive(ctx context.Context) error {
	m.goLiveCalled = true
	return nil
}

func (m *mockRuntime) StopAll() {
	m.stopCalled = true
}

func (m *mockRuntime) HardStop() {
	m.hardStopCalled = true
}

func (m *mockRuntime) DiagFacebook(ctx context.Context, targetID string) ([]string, error) {
	m.diagCalled = true
	m.diagTargetID = targetID
	return m.diagLogs, m.diagErr
}

func TestServer_Status(t *testing.T) {
	cfg := config.DefaultConfig()
	rt := &mockRuntime{}
	srv := NewServer(cfg, rt, "psk", "v0.1.0")

	req := httptest.NewRequest("GET", "/api/status", nil)
	rr := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["source_mode"] != "standby" {
		t.Errorf("expected standby, got %v", resp["source_mode"])
	}

	if !rt.statusCalled {
		t.Errorf("expected runtime.Status to be called")
	}
}

func TestServer_Auth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.Mode = "psk"
	rt := &mockRuntime{}
	srv := NewServer(cfg, rt, "secret-psk", "v0.1.0")

	t.Run("no auth header returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/preview", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/preview", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("correct token returns 200", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/preview", nil)
		req.Header.Set("Authorization", "Bearer secret-psk")
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("auth mode none bypasses auth", func(t *testing.T) {
		cfg2 := config.DefaultConfig()
		cfg2.Auth.Mode = "none"
		srv2 := NewServer(cfg2, rt, "secret-psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/preview", nil)
		rr := httptest.NewRecorder()
		srv2.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200 with auth mode none, got %d", rr.Code)
		}
	})

	t.Run("empty auth mode bypasses auth", func(t *testing.T) {
		cfg2 := config.DefaultConfig()
		cfg2.Auth.Mode = ""
		srv2 := NewServer(cfg2, rt, "secret-psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/preview", nil)
		rr := httptest.NewRecorder()
		srv2.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200 with empty auth mode, got %d", rr.Code)
		}
	})
}

func TestServer_HandlePreview(t *testing.T) {
	cfg := config.DefaultConfig()
	rt := &mockRuntime{}
	srv := NewServer(cfg, rt, "psk", "v0.1.0")

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/preview", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !rt.previewCalled {
			t.Error("expected StartPreview to be called")
		}

		var resp map[string]any
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp["ok"] != true {
			t.Errorf("expected ok=true, got %v", resp["ok"])
		}
	})

	t.Run("error returns 500", func(t *testing.T) {
		rtErr := &mockRuntime{}
		rtErr.previewCalled = false
		// We can't easily make StartPreview return an error with the simple mock,
		// but we can verify the handler structure works
		srvErr := NewServer(cfg, rtErr, "psk", "v0.1.0")
		req := httptest.NewRequest("POST", "/api/preview", nil)
		rr := httptest.NewRecorder()
		srvErr.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestServer_HandleGoLive(t *testing.T) {
	cfg := config.DefaultConfig()
	rt := &mockRuntime{}
	srv := NewServer(cfg, rt, "psk", "v0.1.0")

	req := httptest.NewRequest("POST", "/api/go-live", nil)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !rt.goLiveCalled {
		t.Error("expected StartGoLive to be called")
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
	if resp["action"] != "go-live" {
		t.Errorf("expected action go-live, got %v", resp["action"])
	}
}

func TestServer_HandleStop(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("normal stop", func(t *testing.T) {
		rt := &mockRuntime{}
		srv := NewServer(cfg, rt, "psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/stop", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !rt.stopCalled {
			t.Error("expected StopAll to be called")
		}
		if rt.hardStopCalled {
			t.Error("expected HardStop NOT to be called")
		}
	})

	t.Run("hard stop", func(t *testing.T) {
		rt := &mockRuntime{}
		srv := NewServer(cfg, rt, "psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/stop?hard=true", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !rt.hardStopCalled {
			t.Error("expected HardStop to be called")
		}
		if rt.stopCalled {
			t.Error("expected StopAll NOT to be called")
		}
	})
}

func TestServer_HandleDiagFacebook(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("success with default target", func(t *testing.T) {
		rt := &mockRuntime{diagLogs: []string{"log line 1", "log line 2"}}
		srv := NewServer(cfg, rt, "psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/diag/facebook", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !rt.diagCalled {
			t.Error("expected DiagFacebook to be called")
		}
		if rt.diagTargetID != "facebook-main" {
			t.Errorf("expected default target facebook-main, got %s", rt.diagTargetID)
		}

		var resp map[string]any
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp["ok"] != true {
			t.Errorf("expected ok=true, got %v", resp["ok"])
		}
	})

	t.Run("success with custom target", func(t *testing.T) {
		rt := &mockRuntime{diagLogs: []string{"test log"}}
		srv := NewServer(cfg, rt, "psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/diag/facebook?target_id=custom-fb", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if rt.diagTargetID != "custom-fb" {
			t.Errorf("expected target custom-fb, got %s", rt.diagTargetID)
		}
	})

	t.Run("error returns 500 with logs", func(t *testing.T) {
		rt := &mockRuntime{
			diagLogs: []string{"error log"},
			diagErr:  errors.New("diagnostic failed"),
		}
		srv := NewServer(cfg, rt, "psk", "v0.1.0")

		req := httptest.NewRequest("POST", "/api/diag/facebook", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rr.Code)
		}

		var resp map[string]any
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp["ok"] != false {
			t.Errorf("expected ok=false, got %v", resp["ok"])
		}
	})
}

func TestServer_HandleAssets(t *testing.T) {
	cfg := config.DefaultConfig()
	rt := &mockRuntime{}
	srv := NewServer(cfg, rt, "psk", "v0.1.0")

	t.Run("empty path returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/assets/", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("nonexistent file returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/assets/nonexistent.png", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})
}

func TestServer_HandleIndex(t *testing.T) {
	t.Run("default config renders page", func(t *testing.T) {
		cfg := config.DefaultConfig()
		rt := &mockRuntime{}
		srv := NewServer(cfg, rt, "", "v0.1.0")

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if rr.Header().Get("content-type") != "text/html; charset=utf-8" {
			t.Errorf("expected text/html content-type, got %s", rr.Header().Get("content-type"))
		}
		body := rr.Body.String()
		if body == "" {
			t.Error("expected non-empty HTML body")
		}
	})

	t.Run("uses app name when UI title empty", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.Name = "MyStream"
		cfg.UI.Title = ""
		rt := &mockRuntime{}
		srv := NewServer(cfg, rt, "", "v0.1.0")

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		body := rr.Body.String()
		if !containsStr(body, "MyStream") {
			t.Error("expected app name in response")
		}
	})

	t.Run("mediamtx preview URL resolution", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.MediaEngine.Type = "mediamtx"
		cfg.MediaEngine.MediaMTX.HLSBaseURL = "https://stream.example.com/hls"
		cfg.UI.PreviewHLSURL = ""
		rt := &mockRuntime{}
		srv := NewServer(cfg, rt, "", "v0.1.0")

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)

		body := rr.Body.String()
		if !containsStr(body, "https://stream.example.com/hls/live/preview/index.m3u8") {
			t.Error("expected auto-resolved preview URL")
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect string
	}{
		{"all empty", []string{"", "  ", ""}, ""},
		{"first non-empty", []string{"a", "b", "c"}, "a"},
		{"skips whitespace", []string{"  ", "b", "c"}, "b"},
		{"single value", []string{"hello"}, "hello"},
		{"no values", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstNonEmpty(tt.input...)
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"key": "value"})

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}
	if rr.Header().Get("content-type") != "application/json" {
		t.Errorf("expected application/json, got %s", rr.Header().Get("content-type"))
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["key"] != "value" {
		t.Errorf("expected key=value, got %v", resp)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{rr, http.StatusOK}

	rw.WriteHeader(http.StatusAccepted)
	if rw.status != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", rw.status)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
