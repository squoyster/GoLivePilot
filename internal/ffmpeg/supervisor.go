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
	exitedCh    chan struct{}
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

	redactedArgs := make([]string, len(args))
	copy(redactedArgs, args)
	for i, arg := range redactedArgs {
		if strings.HasPrefix(arg, "rtmp") {
			redactedArgs[i] = RedactURL(arg)
		}
	}
	slog.Info("supervisor: final ffmpeg args", "binary", req.Binary, "args", strings.Join(redactedArgs, " "))

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
		logs:     NewRingLog(500),
		logger:   slog.With("target_id", req.TargetID, "mode", req.Mode),
		exitedCh: make(chan struct{}),
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
	go s.monitor(req.TargetID, rp)

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

	// Wait for the monitor goroutine to detect exit, with timeout and force-kill fallback.
	select {
	case <-rp.exitedCh:
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Wait for monitor to process the kill
		select {
		case <-rp.exitedCh:
		case <-time.After(500 * time.Millisecond):
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	delete(s.relays, targetID)
	s.mu.Unlock()

	return nil
}

func (s *ProcessSupervisor) monitor(targetID string, rp *relayProcess) {
	err := rp.cmd.Wait()
	rp.logger.Info("ffmpeg exited", "error", err)

	s.mu.Lock()
	if err != nil {
		rp.status.LastError = RedactURL(err.Error())
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
		s.mu.Unlock()
		close(rp.exitedCh)
		return
	}

	delete(s.relays, targetID)
	s.mu.Unlock()

	close(rp.exitedCh)
}

func (s *ProcessSupervisor) Switch(ctx context.Context, req StartRequest) error {
	// Stop the old relay first, then start the new one.
	// The brief gap between stop and start is acceptable for v0.1.
	// Future versions can implement start-before-stop using MediaMTX's
	// overridePublisher feature once it's configured.
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
	for id, st := range s.lastStatus {
		out[id] = st
	}
	for id, rp := range s.relays {
		st := rp.status
		if rp.logs != nil {
			st.Logs = rp.logs.Lines()
		}
		out[id] = st
	}
	return out
}

func RedactURL(u string) string {
	if !strings.Contains(u, "rtmp") {
		return u
	}
	if idx := strings.LastIndex(u, "/"); idx != -1 {
		return u[:idx+1] + "****"
	}
	return u
}