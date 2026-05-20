package streamswitch

import (
	"context"
	"testing"
	"time"
)

func TestStreamSwitch_NewAndStart(t *testing.T) {
	cfg := Config{
		Inputs: []InputConfig{
			{
				ID:   SourceSlate,
				Args: []string{"-re", "-f", "lavfi", "-i", "color=c=black:s=1920x1080:d=1", "-c:v", "libx264", "-preset", "ultrafast", "-f", "flv"},
			},
			{
				ID:   SourceCamera,
				Args: []string{"-re", "-f", "lavfi", "-i", "color=c=red:s=1920x1080:d=1", "-c:v", "libx264", "-preset", "ultrafast", "-f", "flv"},
			},
		},
		Outputs: []OutputConfig{
			{
				TargetID:   "preview",
				URL:        "rtmp://localhost:1935/live/preview",
				EncodeArgs: []string{"-c", "copy"},
			},
		},
		StartTimeout: 10 * time.Second,
	}

	ss, err := NewStreamSwitch(cfg)
	if err != nil {
		t.Fatalf("NewStreamSwitch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = ss.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if ss.Active() != SourceSlate {
		t.Errorf("expected active slate, got %s", ss.Active())
	}

	ss.Stop()
}

func TestStreamSwitch_Switch(t *testing.T) {
	cfg := Config{
		Inputs: []InputConfig{
			{
				ID:   SourceSlate,
				Args: []string{"-re", "-f", "lavfi", "-i", "color=c=black:s=1920x1080:d=2", "-c:v", "libx264", "-preset", "ultrafast", "-f", "flv"},
			},
			{
				ID:   SourceCamera,
				Args: []string{"-re", "-f", "lavfi", "-i", "color=c=red:s=1920x1080:d=2", "-c:v", "libx264", "-preset", "ultrafast", "-f", "flv"},
			},
		},
		Outputs: []OutputConfig{
			{
				TargetID:   "preview",
				URL:        "rtmp://localhost:1935/live/preview",
				EncodeArgs: []string{"-c", "copy"},
			},
		},
		StartTimeout: 10 * time.Second,
	}

	ss, err := NewStreamSwitch(cfg)
	if err != nil {
		t.Fatalf("NewStreamSwitch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := ss.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if ss.Active() != SourceSlate {
		t.Errorf("expected active slate, got %s", ss.Active())
	}

	// Let the first input start producing
	time.Sleep(500 * time.Millisecond)

	// Switch to camera
	if err := ss.Switch(ctx, SourceCamera); err != nil {
		t.Fatalf("Switch to camera: %v", err)
	}

	if ss.Active() != SourceCamera {
		t.Errorf("expected active camera after switch, got %s", ss.Active())
	}

	// Let it run for a moment
	time.Sleep(500 * time.Millisecond)

	ss.Stop()
}

func TestStreamSwitch_NoSuchInput(t *testing.T) {
	cfg := Config{
		Inputs: []InputConfig{
			{ID: SourceSlate, Args: []string{"-re", "-f", "lavfi", "-i", "color=c=black:d=1", "-f", "flv"}},
		},
		Outputs: []OutputConfig{
			{TargetID: "preview", URL: "rtmp://localhost:1935/live/preview"},
		},
	}

	ss, err := NewStreamSwitch(cfg)
	if err != nil {
		t.Fatalf("NewStreamSwitch: %v", err)
	}

	ctx := context.Background()
	if err := ss.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err = ss.Switch(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent input")
	}

	ss.Stop()
}

func TestStreamSwitch_NoInputs(t *testing.T) {
	_, err := NewStreamSwitch(Config{
		Inputs:  nil,
		Outputs: []OutputConfig{{TargetID: "preview", URL: "rtmp://localhost/preview"}},
	})
	if err == nil {
		t.Error("expected error for no inputs")
	}
}

func TestStreamSwitch_NoOutputs(t *testing.T) {
	_, err := NewStreamSwitch(Config{
		Inputs:  []InputConfig{{ID: SourceSlate, Args: []string{"-v", "quiet", "-f", "null", "-"}}},
		Outputs: nil,
	})
	if err == nil {
		t.Error("expected error for no outputs")
	}
}