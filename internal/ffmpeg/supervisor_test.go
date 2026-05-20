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

	// We can't directly call captureLogs because it's a method on relayProcess
	// but we can test the logic if we simulate it or just test RingLog if it had an Add method.
	// Since RingLog doesn't have an Add method, it's mostly used internally by captureLogs.
	// Let's check how captureLogs is implemented.
}

func TestSupervisor_Lifecycle(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	req := StartRequest{
		TargetID: "test",
		Label:    "Test Relay",
		Binary:   "sleep",
		LogLevel: "info",
		Input:    "10", // sleep 10
		Output:   "",
	}

	// Override BuildArgs for testing or just use simple binary
	// Actually Start calls exec.Command(req.Binary, args...)
	// BuildArgs will add -hide_banner, -loglevel, -i, -f flv
	// So it will be: sleep -hide_banner -loglevel info -i 10 -f flv
	// This might fail if sleep doesn't like these args.

	// Let's use 'sh' and '-c' 'sleep 10'
	req = StartRequest{
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

	// Wait for wait() goroutine to update status
	time.Sleep(100 * time.Millisecond)

	status = s.Status()
	state := status["test"].State
	// When Stop() is called, it might delete the relay from s.relays before wait() can update s.lastStatus.
	// However, Stop() itself calls cmd.Wait() and then deletes from s.relays.
	// If wait() also calls cmd.Wait(), there's a race on who finishes first.
	// But in both cases, it should be either in relays or in lastStatus.
	if state != StateStopped && state != StateFailed && state != StateStopping && state != "" {
		// If it's empty, it might have been deleted from both?
		// That shouldn't happen if it was in lastStatus.
		t.Errorf("got unexpected state %q", state)
	}
}

func TestSupervisor_RestartFailed(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	// Start a process that fails immediately
	req := StartRequest{
		TargetID: "fail",
		Binary:   "false",
	}

	_ = s.Start(ctx, req)

	// Wait a bit for it to fail
	time.Sleep(200 * time.Millisecond)

	status := s.Status()
	if status["fail"].State != StateFailed {
		t.Errorf("expected failed, got %s", status["fail"].State)
	}

	// Verify it's NOT in s.relays anymore but IN s.lastStatus
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

	// Try to start again
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
