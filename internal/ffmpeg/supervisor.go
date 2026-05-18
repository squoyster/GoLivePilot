package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var ErrAlreadyRunning = errors.New("relay already running")
var ErrNotRunning = errors.New("relay not running")

type ProcessSupervisor struct {
	mu     sync.Mutex
	relays map[string]*relayProcess
}

type relayProcess struct {
	req         StartRequest
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	status      RelayStatus
	intentional bool
	logs        *RingLog
}

func NewSupervisor() *ProcessSupervisor {
	return &ProcessSupervisor{
		relays: make(map[string]*relayProcess),
	}
}

func (s *ProcessSupervisor) Start(ctx context.Context, req StartRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.TargetID == "" {
		return fmt.Errorf("target id is required")
	}
	if _, exists := s.relays[req.TargetID]; exists {
		return ErrAlreadyRunning
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
		logs: NewRingLog(500),
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

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
	s.mu.Lock()
	rp, ok := s.relays[targetID]
	if !ok {
		s.mu.Unlock()
		return ErrNotRunning
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
	case <-time.After(5 * time.Second):
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

	out := make(map[string]RelayStatus, len(s.relays))
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

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.relays[targetID]
	if !ok || current != rp {
		return
	}

	if rp.intentional {
		delete(s.relays, targetID)
		return
	}

	rp.status.State = StateFailed
	if err != nil {
		rp.status.LastError = err.Error()
	}

	// Reconnect loop comes next. For first version, leave failed.
}
