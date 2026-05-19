package ffmpeg

import (
	"bufio"
	"io"
	"sync"
)

type RingLog struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func NewRingLog(max int) *RingLog {
	return &RingLog{
		lines: make([]string, 0, max),
		max:   max,
	}
}

func (r *RingLog) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func (rp *relayProcess) captureLogs(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			rp.logs.mu.Lock()
			if len(rp.logs.lines) >= rp.logs.max {
				rp.logs.lines = rp.logs.lines[1:]
			}
			rp.logs.lines = append(rp.logs.lines, line)
			rp.logs.mu.Unlock()
			rp.logger.Debug("ffmpeg log", "line", line)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		rp.logger.Error("log scanner error", "error", err)
	}
}
