package ffmpeg

import (
	"context"
	"testing"
	"time"
)

func TestRingLog(t *testing.T) {
	rl := NewRingLog(3)
	if len(rl.Lines()) != 0 {
		t.Errorf("expected empty lines, got %d", len(rl.Lines()))
	}
}

func TestSupervisor_Lifecycle(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req := StartRequest{
		TargetID:  "test",
		Label:     "Test Relay",
		Binary:    "sh",
		LogLevel:  "info",
		InputArgs: []string{"-c", "sleep 1"},
		Output:    "",
	}

	err := s.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	status := s.Status()
	if status["test"].State != StateRunning && status["test"].State != StateStarting {
		t.Errorf("expected running or starting, got %s", status["test"].State)
	}

	err = s.Stop(ctx, "test")
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	status = s.Status()
	state := status["test"].State
	if state != StateStopped && state != StateFailed && state != StateStopping && state != "" {
		t.Errorf("got unexpected state %q", state)
	}
}

func TestSupervisor_RestartFailed(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req := StartRequest{
		TargetID: "fail",
		Binary:   "false",
	}

	_ = s.Start(ctx, req)

	time.Sleep(200 * time.Millisecond)

	status := s.Status()
	if status["fail"].State != StateFailed {
		t.Errorf("expected failed, got %s", status["fail"].State)
	}

	s.mu.Lock()
	_, inRelays := s.relays["fail"]
	_, inLast := s.lastStatus["fail"]
	s.mu.Unlock()

	if inRelays {
		t.Error("expected 'fail' to be removed from active relays after exit")
	}
	if !inLast {
		t.Error("expected 'fail' to be in lastStatus after exit")
	}

	req.Binary = "sleep"
	req.InputArgs = []string{"1"}
	err := s.Start(ctx, req)
	if err != nil {
		t.Errorf("Restart failed: %v", err)
	}

	status = s.Status()
	if status["fail"].State != StateRunning && status["fail"].State != StateStarting {
		t.Errorf("expected running/starting after restart, got %s", status["fail"].State)
	}

	s.Stop(ctx, "fail")
}

func TestSupervisor_Switch(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	t.Run("switches running relay", func(t *testing.T) {
		req1 := StartRequest{
			TargetID:  "relay",
			Label:     "First",
			Binary:    "sh",
			LogLevel:  "info",
			InputArgs: []string{"-c", "sleep 10"},
		}
		if err := s.Start(ctx, req1); err != nil {
			t.Fatalf("first start failed: %v", err)
		}

		status := s.Status()
		if status["relay"].State != StateRunning && status["relay"].State != StateStarting {
			t.Errorf("expected running, got %s", status["relay"].State)
		}

		req2 := StartRequest{
			TargetID:  "relay",
			Label:     "Second",
			Binary:    "sh",
			LogLevel:  "info",
			InputArgs: []string{"-c", "sleep 10"},
		}
		if err := s.Switch(ctx, req2); err != nil {
			t.Fatalf("switch failed: %v", err)
		}

		status = s.Status()
		if status["relay"].State != StateRunning && status["relay"].State != StateStarting {
			t.Errorf("expected running after switch, got %s", status["relay"].State)
		}

		s.Stop(ctx, "relay")
	})

	t.Run("switches non-running relay", func(t *testing.T) {
		req := StartRequest{
			TargetID:  "new-relay",
			Label:     "New",
			Binary:    "sh",
			LogLevel:  "info",
			InputArgs: []string{"-c", "sleep 1"},
		}
		if err := s.Switch(ctx, req); err != nil {
			t.Fatalf("switch failed: %v", err)
		}

		status := s.Status()
		if status["new-relay"].State != StateRunning && status["new-relay"].State != StateStarting {
			t.Errorf("expected running, got %s", status["new-relay"].State)
		}

		s.Stop(ctx, "new-relay")
	})
}

func TestSupervisor_StopAll(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req1 := StartRequest{
		TargetID:  "relay-a",
		Binary:    "sh",
		LogLevel:  "info",
		InputArgs: []string{"-c", "sleep 10"},
		Output:    "",
	}
	req2 := StartRequest{
		TargetID:  "relay-b",
		Binary:    "sh",
		LogLevel:  "info",
		InputArgs: []string{"-c", "sleep 10"},
		Output:    "",
	}

	_ = s.Start(ctx, req1)
	_ = s.Start(ctx, req2)

	s.StopAll(ctx)

	time.Sleep(200 * time.Millisecond)

	status := s.Status()
	for _, id := range []string{"relay-a", "relay-b"} {
		if st, ok := status[id]; ok {
			if st.State == StateRunning {
				t.Errorf("expected %s to be stopped, got %s", id, st.State)
			}
		}
	}
}

func TestSupervisor_Stop_NotRunning(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	err := s.Stop(ctx, "nonexistent")
	if err != ErrNotRunning {
		t.Errorf("expected ErrNotRunning, got %v", err)
	}
}

func TestSupervisor_Start_DuplicateRunning(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req := StartRequest{
		TargetID:  "dup",
		Binary:    "sh",
		LogLevel:  "info",
		InputArgs: []string{"-c", "sleep 10"},
		Output:    "",
	}

	if err := s.Start(ctx, req); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	err := s.Start(ctx, req)
	if err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}

	s.Stop(ctx, "dup")
}

func TestSupervisor_Start_EmptyTargetID(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req := StartRequest{
		TargetID: "",
	}

	err := s.Start(ctx, req)
	if err == nil {
		t.Fatal("expected error for empty target ID")
	}
}

func TestSupervisor_Status(t *testing.T) {
	s := NewSupervisor()

	status := s.Status()
	if len(status) != 0 {
		t.Errorf("expected empty status, got %d entries", len(status))
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"non-rtmp url", "http://example.com/path", "http://example.com/path"},
		{"rtmp url no key", "rtmp://localhost:1935/live/", "rtmp://localhost:1935/live/****"},
		{"rtmps url with key", "rtmps://live-api-s.facebook.com:443/rtmp/FB-KEY-123", "rtmps://live-api-s.facebook.com:443/rtmp/****"},
		{"rtmp url without trailing slash", "rtmp://localhost:1935/live", "rtmp://localhost:1935/****"},
		{"empty string", "", ""},
		{"rtmp without slash", "rtmpvalue", "rtmpvalue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactURL(tt.input)
			if result != tt.expect {
				t.Errorf("RedactURL(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestBuildArgs_SlateModeExtended(t *testing.T) {
	req := StartRequest{
		TargetID: "slate",
		Binary:   "ffmpeg",
		LogLevel: "debug",
		Input:    "assets/starting-soon.png",
		Output:   "rtmp://localhost:1935/live/preview",
		InputArgs: []string{
			"-re", "-loop", "1", "-framerate", "30",
			"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		},
		OutputArgs: []string{
			"-map", "0:v:0", "-map", "1:a:0",
			"-c:v", "libx264", "-preset", "veryfast",
			"-tune", "stillimage", "-pix_fmt", "yuv420p",
			"-r", "30", "-g", "60", "-b:v", "3000k",
			"-maxrate", "3000k", "-bufsize", "6000k",
			"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		},
	}

	args, err := BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	found := false
	for _, a := range args {
		if a == "libx264" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected libx264 in args")
	}

	lastArg := args[len(args)-1]
	if lastArg != "rtmp://localhost:1935/live/preview" {
		t.Errorf("expected output URL as last arg, got %s", lastArg)
	}
}

func TestBuildArgs_Defaults(t *testing.T) {
	req := StartRequest{
		TargetID: "test",
		Input:    "rtmp://localhost/input",
		Output:   "rtmp://localhost/output",
	}

	args, err := BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	if args[0] != "-hide_banner" {
		t.Errorf("expected -hide_banner first, got %s", args[0])
	}

	foundFLV := false
	for i, a := range args {
		if a == "-f" && i+1 < len(args) && args[i+1] == "flv" {
			foundFLV = true
			break
		}
	}
	if !foundFLV {
		t.Error("expected -f flv in args")
	}
}

func TestBuildArgs_NoDuplicateFLV(t *testing.T) {
	req := StartRequest{
		TargetID:   "test",
		Input:      "rtmp://localhost/input",
		Output:     "rtmp://localhost/output",
		OutputArgs: []string{"-f", "flv", "-c", "copy"},
	}

	args, err := BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	flvCount := 0
	for i, a := range args {
		if a == "-f" && i+1 < len(args) && args[i+1] == "flv" {
			flvCount++
		}
	}
	if flvCount != 1 {
		t.Errorf("expected exactly one -f flv, got %d", flvCount)
	}
}

func TestBuildArgs_InputAlreadyInInputArgs(t *testing.T) {
	req := StartRequest{
		TargetID:  "test",
		Input:     "test-input",
		InputArgs: []string{"-i", "test-input"},
		Output:    "rtmp://localhost/output",
	}

	args, err := BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	inputCount := 0
	for i, a := range args {
		if a == "-i" && i+1 < len(args) && args[i+1] == "test-input" {
			inputCount++
		}
	}
	if inputCount != 1 {
		t.Errorf("expected exactly one -i test-input, got %d", inputCount)
	}
}
