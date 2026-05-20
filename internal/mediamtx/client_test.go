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

func TestClient_GetPath_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		resp := pathsListResponse{
			Items: []PathInfo{},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPath(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestClient_GetPath_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPath(context.Background(), "live/program")
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestClient_GetPath_ConnectionError(t *testing.T) {
	client := NewClient("http://localhost:19999")
	_, err := client.GetPath(context.Background(), "live/program")
	if err == nil {
		t.Fatal("expected error for connection refused")
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

func TestClient_WaitUntilHealthy(t *testing.T) {
	t.Run("healthy immediately", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
			resp := pathsListResponse{Items: []PathInfo{}}
			json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient(server.URL)
		err := client.WaitUntilHealthy(context.Background(), 2*time.Second)
		if err != nil {
			t.Fatalf("WaitUntilHealthy failed: %v", err)
		}
	})

	t.Run("becomes healthy after retries", func(t *testing.T) {
		callCount := 0
		mux := http.NewServeMux()
		mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 3 {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			resp := pathsListResponse{Items: []PathInfo{}}
			json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient(server.URL)
		err := client.WaitUntilHealthy(context.Background(), 5*time.Second)
		if err != nil {
			t.Fatalf("WaitUntilHealthy failed: %v", err)
		}
		if callCount < 3 {
			t.Errorf("expected at least 3 calls, got %d", callCount)
		}
	})

	t.Run("timeout when never healthy", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient(server.URL)
		err := client.WaitUntilHealthy(context.Background(), 500*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})
}

func TestClient_NewClient(t *testing.T) {
	client := NewClient("http://localhost:9997")
	if client.BaseURL != "http://localhost:9997" {
		t.Errorf("expected baseURL http://localhost:9997, got %s", client.BaseURL)
	}
	if client.HTTPClient == nil {
		t.Error("expected HTTPClient to be set")
	}
	if client.HTTPClient.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", client.HTTPClient.Timeout)
	}
}

func TestClient_WaitForPathReady_ContextCancelled(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := client.WaitForPathReady(ctx, "live/program", 5*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestClient_WaitUntilHealthy_ContextCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := client.WaitUntilHealthy(ctx, 5*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}
