package ffmpeg

import (
	"io"
	"strings"
	"sync"
)

type RingLog struct {
	mu     sync.Mutex
	lines  []string
	max    int
	cursor int
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
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := strings.TrimSpace(string(buf[:n]))
			if line != "" {
				rp.logs.mu.Lock()
				if len(rp.logs.lines) >= rp.logs.max {
					rp.logs.lines = rp.logs.lines[1:]
				}
				rp.logs.lines = append(rp.logs.lines, line)
				rp.logs.mu.Unlock()
			}
		}
		if err != nil {
			break
		}
	}
}
