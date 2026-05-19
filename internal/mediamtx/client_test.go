package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_GetPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		resp := pathsListResponse{
			Items: []PathInfo{
				{Name: "live/program", Ready: true, Available: true, Online: true, Tracks: []string{"h264"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	info, err := client.GetPath(context.Background(), "live/program")
	if err != nil {
		t.Fatalf("GetPath failed: %v", err)
	}

	if info.Name != "live/program" {
		t.Errorf("expected live/program, got %s", info.Name)
	}
	if !info.Ready {
		t.Errorf("expected ready")
	}
}

func TestClient_WaitForPathReady(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		ready := false
		if callCount >= 3 {
			ready = true
		}
		resp := pathsListResponse{
			Items: []PathInfo{
				{Name: "live/program", Ready: ready, Available: true, Online: true, Tracks: []string{"h264"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	// Reduce polling interval for faster test if possible, but WaitForPathReady uses 250ms hardcoded.
	// So we just wait.
	info, err := client.WaitForPathReady(context.Background(), "live/program", 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForPathReady failed: %v", err)
	}

	if !info.Ready {
		t.Errorf("expected ready")
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestClient_WaitForPathReady_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		resp := pathsListResponse{
			Items: []PathInfo{
				{Name: "live/program", Ready: false, Available: true, Online: true, Tracks: []string{"h264"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.WaitForPathReady(context.Background(), "live/program", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
