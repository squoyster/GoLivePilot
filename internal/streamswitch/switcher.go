package streamswitch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// StreamSwitch manages multiple input sources and routes the selected one
// to all output streams. Outputs persist for the entire session.
type StreamSwitch struct {
	cfg     Config
	logger  *slog.Logger

	inputs       map[SourceID]*InputSource
	outputs      []*OutputStream
	active       SourceID
	lastGOPSeq   map[SourceID]uint64 // Last sent GOP seq per input

	mu         sync.RWMutex
	started    bool
	forwardCtx context.Context
	stopFunc   context.CancelFunc
	forwardWg  sync.WaitGroup
}

// NewStreamSwitch creates a stream switch from the given config.
func NewStreamSwitch(cfg Config) (*StreamSwitch, error) {
	if len(cfg.Inputs) == 0 {
		return nil, fmt.Errorf("streamswitch: at least one input required")
	}
	if len(cfg.Outputs) == 0 {
		return nil, fmt.Errorf("streamswitch: at least one output required")
	}
	if cfg.StartTimeout == 0 {
		cfg.StartTimeout = 15 * time.Second
	}

	ss := &StreamSwitch{
		cfg:        cfg,
		logger:     slog.With("component", "streamswitch"),
		inputs:     make(map[SourceID]*InputSource),
		active:     SourceUnknown,
		lastGOPSeq: make(map[SourceID]uint64),
	}

	for _, ic := range cfg.Inputs {
		ss.inputs[ic.ID] = NewInputSource(ic)
	}

	for _, oc := range cfg.Outputs {
		ss.outputs = append(ss.outputs, NewOutputStream(oc))
	}

	return ss, nil
}

// Start starts all input sources and output streams.
// The first input in the config is selected as the active source.
func (ss *StreamSwitch) Start(ctx context.Context) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.started {
		return fmt.Errorf("streamswitch already started")
	}

	ss.logger.Info("starting streamswitch",
		"inputs", len(ss.inputs),
		"outputs", len(ss.outputs),
	)

	// Start FFmpeg subs for each output
	for _, out := range ss.outputs {
		if err := out.Start(ctx); err != nil {
			ss.cleanup()
			return fmt.Errorf("start output %s: %w", out.cfg.TargetID, err)
		}
	}

	// Write FLV headers to all outputs
	for _, out := range ss.outputs {
		if err := out.WriteHeader(true, true); err != nil {
			ss.cleanup()
			return fmt.Errorf("write header to %s: %w", out.cfg.TargetID, err)
		}
	}

	// Start input sources
	for _, in := range ss.inputs {
		if err := in.Start(ctx); err != nil {
			ss.cleanup()
			return fmt.Errorf("start input %s: %w", in.cfg.ID, err)
		}
	}

	// Set active to first input
	if len(ss.cfg.Inputs) > 0 {
		ss.active = ss.cfg.Inputs[0].ID
	}

	// Start the forwarding goroutine
	ss.forwardCtx, ss.stopFunc = context.WithCancel(context.Background())
	ss.forwardWg.Add(1)
	go ss.forwardLoop(ss.forwardCtx)

	ss.started = true
	ss.logger.Info("streamswitch started", "active", ss.active)
	return nil
}

// forwardLoop continuously reads GOPs from the active input and writes them to all outputs.
func (ss *StreamSwitch) forwardLoop(ctx context.Context) {
	defer ss.forwardWg.Done()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ss.forwardTick()
		}
	}
}

// forwardTick reads the current GOP from the active input and forwards to all outputs.
func (ss *StreamSwitch) forwardTick() {
	ss.mu.RLock()
	active := ss.active
	inputs := ss.inputs
	outputs := ss.outputs
	ss.mu.RUnlock()

	if active == SourceUnknown {
		return
	}

	in, ok := inputs[active]
	if !ok {
		return
	}

	gop, seq := in.CurrentGOP()
	if len(gop.Tags) == 0 {
		return
	}

	// Only forward if this is a new GOP we haven't sent before
	ss.mu.RLock()
	lastSeq := ss.lastGOPSeq[active]
	ss.mu.RUnlock()
	if seq <= lastSeq {
		return
	}

	for _, out := range outputs {
		ptsOff := out.PTSOffset()
		if err := out.WriteGOP(gop, ptsOff, false); err != nil {
			ss.logger.Warn("write gop failed", "target", out.cfg.TargetID, "error", err)
		}
	}

	ss.mu.Lock()
	ss.lastGOPSeq[active] = seq
	ss.mu.Unlock()
}

// Switch selects the active input source. All outputs seamlessly transition
// at the next GOP boundary. Timestamps are adjusted for continuity.
func (ss *StreamSwitch) Switch(ctx context.Context, target SourceID) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.started {
		return fmt.Errorf("streamswitch not started")
	}
	if ss.active == target {
		return nil // no-op
	}

	newIn, ok := ss.inputs[target]
	if !ok {
		return fmt.Errorf("input %s not found", target)
	}

	// Get the last PTS from the current active output state
	var lastPTS uint32
	for _, out := range ss.outputs {
		lp := out.LastPTS()
		if lp > lastPTS {
			lastPTS = lp
		}
	}

	// Get a GOP from the new input to ensure it has data
	gop, _ := newIn.CurrentGOP()
	if len(gop.Tags) == 0 {
		// Wait briefly for the new input to produce a GOP
		for i := 0; i < 50; i++ { // ~2.5 seconds total
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			time.Sleep(50 * time.Millisecond)
			gop, _ = newIn.CurrentGOP()
			if len(gop.Tags) > 0 {
				break
			}
		}
	}

	// Calculate PTS offset so the new input's first GOP starts at lastPTS + 1
	firstPTS := uint32(0)
	if len(gop.Tags) > 0 {
		firstPTS = gop.Tags[0].Timestamp
	}
	baseOffset := lastPTS + 1
	if firstPTS > baseOffset {
		baseOffset = 0 // firstPTS is already beyond our offset, no adjustment needed
	} else {
		baseOffset = baseOffset - firstPTS
	}

	ss.logger.Info("switching input",
		"from", ss.active,
		"to", target,
		"pts_offset", baseOffset,
		"last_pts", lastPTS,
	)

	// Apply the offset to all outputs and flush the first GOP
	for _, out := range ss.outputs {
		out.SetPTSOffset(baseOffset)
		if len(gop.Tags) > 0 {
			if err := out.WriteGOP(gop, baseOffset, false); err != nil {
				ss.logger.Warn("switch write gop failed", "target", out.cfg.TargetID, "error", err)
			}
		}
	}

	ss.active = target
	ss.logger.Info("input switched", "active", ss.active)
	return nil
}

// Active returns the currently selected input source ID.
func (ss *StreamSwitch) Active() SourceID {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.active
}

// Stop shuts down the stream switch and all its inputs/outputs.
func (ss *StreamSwitch) Stop() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.started {
		return
	}

	ss.logger.Info("stopping streamswitch")

	// Stop forwarding
	if ss.stopFunc != nil {
		ss.stopFunc()
	}
	ss.forwardWg.Wait()

	// Stop inputs
	for _, in := range ss.inputs {
		in.Stop()
	}

	// Close outputs
	for _, out := range ss.outputs {
		_ = out.Close()
	}

	ss.started = false
	ss.logger.Info("streamswitch stopped")
}

func (ss *StreamSwitch) cleanup() {
	for _, in := range ss.inputs {
		in.Stop()
	}
	for _, out := range ss.outputs {
		_ = out.Close()
	}
}