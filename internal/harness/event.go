package harness

import (
	"bytes"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// EventKind tells what a harness saw happen during a turn.
type EventKind string

// Event kinds. Any event counts as a sign of life for the stall rule.
const (
	EventOutput   EventKind = "output"    // the agent wrote text or a tool returned
	EventToolCall EventKind = "tool_call" // the agent called a tool
	EventUsage    EventKind = "usage"     // tokens or money spent since the last usage event
)

// Event is what an adapter tells the supervisor, in the same shape for every harness.
type Event struct {
	Kind  EventKind
	Tool  string // EventToolCall: the tool name
	Input string // EventToolCall: the arguments, as the adapter saw them
	Usage Usage  // EventUsage: the delta, not a running total
}

// Emit passes an event to the observer of the request, if any.
func (r RunRequest) Emit(ev Event) {
	if r.Observe != nil {
		r.Observe(ev)
	}
}

// LineWriter splits what a CLI prints into lines and calls fn for each complete one, so an
// adapter can watch the stream while the process runs. Its Write never fails.
func LineWriter(fn func(line []byte)) *Lines { return &Lines{fn: fn} }

// Lines is the writer LineWriter returns.
type Lines struct {
	mu  sync.Mutex
	fn  func([]byte)
	buf []byte
}

// maxLine bounds the memory kept for one line without a newline.
const maxLine = 16 << 20

// Write implements io.Writer.
func (l *Lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		if i > 0 {
			l.fn(l.buf[:i])
		}
		l.buf = l.buf[i+1:]
	}
	if len(l.buf) > maxLine {
		l.buf = nil
	}
	return len(p), nil
}

// stopGrace is how long a canceled CLI has to stop before its whole process group is killed.
const stopGrace = 5 * time.Second

// StopGroup makes cmd start in its own process group and, when its context ends, ask the whole
// group to stop (SIGTERM) and kill it after a grace period. A CLI that spawns shells or test
// runners would otherwise leave them running after the CLI itself is killed.
func StopGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		time.AfterFunc(stopGrace, func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
		return syscall.Kill(-pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = stopGrace + time.Second
}
