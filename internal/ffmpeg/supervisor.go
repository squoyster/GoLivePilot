package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrAlreadyRunning = errors.New("relay already running")
var ErrNotRunning = errors.New("relay not running")

type ProcessSupervisor struct {
	mu         sync.Mutex
	relays     map[string]*relayProcess
	lastStatus map[string]RelayStatus
}

type relayProcess struct {
	req         StartRequest
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	status      RelayStatus
	intentional bool
	logs        *RingLog
	logger      *slog.Logger
}

func (rp *relayProcess) isExited() bool {
	return rp.cmd != nil && rp.cmd.ProcessState != nil && rp.cmd.ProcessState.Exited()
}

func NewSupervisor() *ProcessSupervisor {
	return &ProcessSupervisor{
		relays:     make(map[string]*relayProcess),
		lastStatus: make(map[string]RelayStatus),
	}
}

func (s *ProcessSupervisor) Start(ctx context.Context, req StartRequest) error {
	slog.Debug("supervisor: starting relay", "target_id", req.TargetID)
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.TargetID == "" {
		return fmt.Errorf("target id is required")
	}

	if existing, exists := s.relays[req.TargetID]; exists {
		if !existing.isExited() && existing.status.State != StateFailed && existing.status.State != StateStopped {
			return ErrAlreadyRunning
		}
		// Clean up existing stale relay
		existing.cancel()
		delete(s.relays, req.TargetID)
	}

	if req.Binary == "" {
		req.Binary = "ffmpeg"
	}
	if req.LogLevel == "" {
		req.LogLevel = "info"
	}

	args, err := BuildArgs(req)
	if err != nil {
		return err
	}
	slog.Info("supervisor: final ffmpeg args", "binary", req.Binary, "args", strings.Join(args, " "))

	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, req.Binary, args...)

	rp := &relayProcess{
		req:    req,
		cmd:    cmd,
		cancel: cancel,
		status: RelayStatus{
			TargetID:  req.TargetID,
			Label:     req.Label,
			Mode:      req.Mode,
			State:     StateStarting,
			StartedAt: time.Now(),
		},
		logs:   NewRingLog(500),
		logger: slog.With("target_id", req.TargetID, "mode", req.Mode),
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		rp.logger.Error("ffmpeg start failed", "error", err)
		cancel()
		return err
	}

	rp.logger.Info("ffmpeg started", "pid", cmd.Process.Pid)
	rp.status.State = StateRunning
	if cmd.Process != nil {
		rp.status.PID = cmd.Process.Pid
	}

	s.relays[req.TargetID] = rp

	go rp.captureLogs(stderr)
	go s.wait(req.TargetID, rp)

	return nil
}

func (s *ProcessSupervisor) Stop(ctx context.Context, targetID string) error {
	slog.Info("supervisor: stopping relay", "target_id", targetID)
	s.mu.Lock()
	rp, ok := s.relays[targetID]
	if !ok {
		s.mu.Unlock()
		return ErrNotRunning
	}

	if rp.isExited() {
		rp.logger.Debug("relay already exited, removing from active map")
		delete(s.relays, targetID)
		s.mu.Unlock()
		return nil
	}

	rp.intentional = true
	rp.status.State = StateStopping
	cmd := rp.cmd
	s.mu.Unlock()

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second): // Reduced from 5s to 2s
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	delete(s.relays, targetID)
	s.mu.Unlock()

	return nil
}

func (s *ProcessSupervisor) Switch(ctx context.Context, req StartRequest) error {
	if err := s.Stop(ctx, req.TargetID); err != nil && !errors.Is(err, ErrNotRunning) {
		return err
	}
	return s.Start(ctx, req)
}

func (s *ProcessSupervisor) StopAll(ctx context.Context) {
	ids := make([]string, 0)

	s.mu.Lock()
	for id := range s.relays {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		_ = s.Stop(ctx, id)
	}
}

func (s *ProcessSupervisor) Status() map[string]RelayStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]RelayStatus, len(s.relays)+len(s.lastStatus))
	// Add historical statuses first
	for id, st := range s.lastStatus {
		out[id] = st
	}
	// Overlay with active statuses
	for id, rp := range s.relays {
		st := rp.status
		if rp.logs != nil {
			st.Logs = rp.logs.Lines()
		}
		out[id] = st
	}
	return out
}

func (s *ProcessSupervisor) wait(targetID string, rp *relayProcess) {
	err := rp.cmd.Wait()
	rp.logger.Info("ffmpeg exited", "error", err)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update status
	if err != nil {
		rp.status.LastError = err.Error()
		rp.status.State = StateFailed
	} else {
		rp.status.State = StateStopped
	}
	if rp.logs != nil {
		rp.status.Logs = rp.logs.Lines()
	}
	s.lastStatus[targetID] = rp.status

	current, ok := s.relays[targetID]
	if !ok || current != rp {
		return
	}

	// Always remove from active relays map once it has exited
	delete(s.relays, targetID)
}
