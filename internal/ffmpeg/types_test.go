package ffmpeg

import (
	"strings"
	"testing"
)

func TestBuildArgs_SlateMode(t *testing.T) {
	req := StartRequest{
		TargetID: "__preview__",
		Label:    "Browser Preview",
		Mode:     "preview",
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
			"-c:v", "libx264", "-preset", "veryfast", "-tune", "stillimage",
			"-pix_fmt", "yuv420p", "-r", "30", "-g", "60", "-b:v", "3000k",
			"-maxrate", "3000k", "-bufsize", "6000k",
			"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		},
	}

	args, err := BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	argStr := strings.Join(args, " ")

	// 1. -loop 1 must appear before its -i
	loopIdx := -1
	iIdx := -1
	for i, arg := range args {
		if arg == "-loop" {
			loopIdx = i
		}
		if arg == "-i" && args[i+1] == req.Input {
			iIdx = i
			break
		}
	}
	if loopIdx == -1 || iIdx == -1 || loopIdx > iIdx {
		t.Errorf("Expected -loop before -i, got indices loop=%d, i=%d", loopIdx, iIdx)
	}

	// 2. -f lavfi must appear before its -i
	lavfiIdx := -1
	lavfiIIdx := -1
	for i, arg := range args {
		if arg == "lavfi" {
			lavfiIdx = i
		}
		if arg == "-i" && strings.HasPrefix(args[i+1], "anullsrc") {
			lavfiIIdx = i
		}
	}
	if lavfiIdx == -1 || lavfiIIdx == -1 || lavfiIdx > lavfiIIdx {
		t.Errorf("Expected -f lavfi before anullsrc -i, got indices lavfi=%d, i=%d", lavfiIdx, lavfiIIdx)
	}

	// 3. Output URL must be the last argument
	if args[len(args)-1] != req.Output {
		t.Errorf("Expected output URL at the end, got %s", args[len(args)-1])
	}

	// 4. No duplicate codec flags (check a few)
	if strings.Count(argStr, "-c:v") > 1 {
		t.Errorf("Duplicate -c:v found: %s", argStr)
	}
	if strings.Count(argStr, "-pix_fmt") > 1 {
		t.Errorf("Duplicate -pix_fmt found: %s", argStr)
	}

	t.Logf("Generated args: %s", argStr)
}
