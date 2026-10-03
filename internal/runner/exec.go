package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// StderrLimit caps the stderr bytes kept in a CommandError. The tail is kept,
// because a command usually prints the cause of a failure last.
const StderrLimit = 8 << 10

// ExecRunner runs commands with os/exec.
type ExecRunner struct {
	// Log, when set, receives the stdout of non-interactive Run calls and a
	// copy of non-interactive stderr. When nil, that output is discarded.
	Log io.Writer
}

var _ Runner = (*ExecRunner)(nil)

// Run executes c. Interactive commands use the terminal. Otherwise stdout
// goes to Log and stderr is captured for the error.
func (r *ExecRunner) Run(ctx context.Context, c Cmd) error {
	// stdout and stderr are copied by separate goroutines, so a shared Log
	// must be serialized.
	log := r.lockedLog()
	var stdout io.Writer
	switch {
	case c.Interactive:
		stdout = os.Stdout
	case log != nil:
		stdout = log
	default:
		stdout = io.Discard
	}
	return runCmd(ctx, c, stdout, log)
}

// Output executes c and returns its stdout.
func (r *ExecRunner) Output(ctx context.Context, c Cmd) ([]byte, error) {
	var buf bytes.Buffer
	if err := runCmd(ctx, c, &buf, r.lockedLog()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *ExecRunner) lockedLog() io.Writer {
	if r.Log == nil {
		return nil
	}
	return &lockedWriter{w: r.Log}
}

// runCmd runs one command. log, when non-nil, receives a copy of
// non-interactive stderr.
func runCmd(ctx context.Context, c Cmd, stdout, log io.Writer) error {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	cmd.Stdin = c.Stdin
	cmd.Stdout = stdout

	stderr := &tailBuffer{limit: StderrLimit}
	switch {
	case c.Interactive:
		if c.Stdin == nil {
			cmd.Stdin = os.Stdin
		}
		cmd.Stderr = os.Stderr
	case log != nil:
		cmd.Stderr = io.MultiWriter(stderr, log)
	default:
		cmd.Stderr = stderr
	}

	err := cmd.Run()
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return &NotFoundError{Name: c.Name}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", c, ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &CommandError{Cmd: c, ExitCode: exitErr.ExitCode(), Stderr: stderr.String()}
	}
	return fmt.Errorf("%s: %w", c, err)
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	limit int
	buf   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.buf) }

// lockedWriter serializes writes to w.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
