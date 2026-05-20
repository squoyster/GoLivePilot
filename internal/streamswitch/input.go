package streamswitch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// InputSource reads FLV from an FFmpeg subprocess and keeps a GOP buffer.
type InputSource struct {
	cfg    InputConfig
	logger *slog.Logger

	cmd       *exec.Cmd
	flvReader *FLVReader
	stdout    io.ReadCloser

	mu        sync.RWMutex
	gop       GOPBuffer          // Last complete GOP
	activeGOP []FLVTag           // Currently building GOP
	started   bool
	stopped   bool

	seqHeaderAAC  []byte // AAC sequence header (AudioSpecificConfig)
	seqHeaderAVC  []byte // AVC sequence header (SPS/PPS)
	hasAudio      bool
	hasVideo      bool
	gopSeq        uint64 // Incremented each time a new GOP is stored
}

// NewInputSource creates a new input source.
func NewInputSource(cfg InputConfig) *InputSource {
	return &InputSource{
		cfg:    cfg,
		logger: slog.With("input", cfg.ID),
	}
}

// Start launches FFmpeg and begins reading FLV tags in a loop.
func (in *InputSource) Start(ctx context.Context) error {
	in.mu.Lock()
	defer in.mu.Unlock()

	if in.started {
		return fmt.Errorf("input %s already started", in.cfg.ID)
	}

	in.started = true
	go in.runLoop(ctx)
	return nil
}

// runLoop manages the FFmpeg subprocess with auto-restart.
func (in *InputSource) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		in.mu.Lock()
		stopped := in.stopped
		in.mu.Unlock()
		if stopped {
			return
		}

		if err := in.startProcess(); err != nil {
			in.logger.Warn("input process failed, retrying in 1s", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		// readLoop returns when FFmpeg exits
		in.readLoop(ctx)
	}
}

func (in *InputSource) startProcess() error {
	args := make([]string, len(in.cfg.Args))
	copy(args, in.cfg.Args)
	args = append(args, "pipe:1")

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	cmd.Stderr = nil // let it go to parent's stderr or /dev/null

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	in.cmd = cmd
	in.stdout = stdout
	in.flvReader = NewFLVReader(bufio.NewReaderSize(stdout, 256*1024))

	return nil
}

func (in *InputSource) readLoop(ctx context.Context) {
	if in.flvReader == nil {
		return
	}

	// Read FLV header
	_, err := in.flvReader.ReadHeader()
	if err != nil {
		in.logger.Warn("flv header read failed", "error", err)
		_ = in.cmd.Wait()
		return
	}

	for {
		select {
		case <-ctx.Done():
			_ = in.killProcess()
			return
		default:
		}

		tag, err := in.flvReader.ReadTag()
		if err != nil {
			in.logger.Debug("flv read ended", "error", err)
			break
		}

		in.processTag(tag)
	}

	_ = in.cmd.Wait()
}

// processTag handles a single FLV tag: updates GOP buffers and tracks codec data.
func (in *InputSource) processTag(tag *FLVTag) {
	in.mu.Lock()
	defer in.mu.Unlock()

	if in.stopped {
		return
	}

	// Track codec sequence headers
	if tag.IsAVCSeqHeader() {
		in.seqHeaderAVC = make([]byte, len(tag.Payload))
		copy(in.seqHeaderAVC, tag.Payload)
		in.hasVideo = true
	}
	if tag.IsAACSeqHeader() {
		in.seqHeaderAAC = make([]byte, len(tag.Payload))
		copy(in.seqHeaderAAC, tag.Payload)
		in.hasAudio = true
	}

	// If this is a keyframe, the PREVIOUS GOP is complete. Swap it out.
	if tag.IsKeyframe() {
		in.gop = GOPBuffer{
			Tags:     in.activeGOP,
			HasAudio: in.hasAudio,
			HasVideo: in.hasVideo,
		}
		in.gopSeq++
		if len(in.activeGOP) > 0 && in.activeGOP[len(in.activeGOP)-1].Timestamp > 0 {
			in.gop.LastPTS = in.activeGOP[len(in.activeGOP)-1].Timestamp
		}
		// Start new GOP with this keyframe
		ts := tag.Timestamp
		in.activeGOP = []FLVTag{
			{
				Type:      tag.Type,
				Timestamp: ts,
				Payload:   tag.Payload,
			},
		}
	} else {
		// Non-keyframe: add to current GOP
		ts := tag.Timestamp
		in.activeGOP = append(in.activeGOP, FLVTag{
			Type:      tag.Type,
			Timestamp: ts,
			Payload:   tag.Payload,
		})
	}
}

// CurrentGOP returns a copy of the last complete GOP buffer and its sequence number.
// Returns the GOP and its sequence number. Callers should track the sequence to
// avoid sending the same GOP multiple times.
func (in *InputSource) CurrentGOP() (GOPBuffer, uint64) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return GOPBuffer{
		Tags:     append([]FLVTag{}, in.gop.Tags...),
		LastPTS:  in.gop.LastPTS,
		HasAudio: in.gop.HasAudio,
		HasVideo: in.gop.HasVideo,
	}, in.gopSeq
}

// SeqHeaders returns copies of the codec sequence headers.
func (in *InputSource) SeqHeaders() (avc, aac []byte) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	if in.seqHeaderAVC != nil {
		avc = make([]byte, len(in.seqHeaderAVC))
		copy(avc, in.seqHeaderAVC)
	}
	if in.seqHeaderAAC != nil {
		aac = make([]byte, len(in.seqHeaderAAC))
		copy(aac, in.seqHeaderAAC)
	}
	return
}

// Stop shuts down the input source.
func (in *InputSource) Stop() {
	in.mu.Lock()
	in.stopped = true
	in.mu.Unlock()
	_ = in.killProcess()
}

func (in *InputSource) killProcess() error {
	if in.cmd != nil && in.cmd.Process != nil {
		return in.cmd.Process.Kill()
	}
	return nil
}