package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/squoyster/golivepilot/internal/config"
)

type mockRuntime struct {
	statusCalled  bool
	previewCalled bool
}

func (m *mockRuntime) Status() map[string]any {
	m.statusCalled = true
	return map[string]any{"source_mode": "standby"}
}

func (m *mockRuntime) StartPreview(ctx context.Context) error {
	m.previewCalled = true
	return nil
}

func (m *mockRuntime) StartGoLive(ctx context.Context) error { return nil }
func (m *mockRuntime) StopAll()                              {}

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

	req := httptest.NewRequest("POST", "/api/preview", nil)
	rr := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rr, req)

	// Should fail with 401 because no auth header
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	// Try with correct PSK
	req = httptest.NewRequest("POST", "/api/preview", nil)
	req.Header.Set("Authorization", "Bearer secret-psk")
	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}
