package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// dryRunner is the Runner of --dry-run.
//
// Invariant: modules use Runner.Output only for read-only queries (brew list,
// git remote get-url, chezmoi diff, ...) and Runner.Run for every command that
// changes the system. So dryRunner prints Run commands as "+ <cmd>" and never
// executes them, but executes non-interactive Output commands through inner
// (and prints them as "? <cmd>"), so Check and Apply see the real state of the
// machine. An interactive Output command is printed and not executed, because
// a command that needs the terminal is never a plain query. A module that
// breaks this invariant breaks --dry-run.
type dryRunner struct {
	inner runner.Runner
	out   io.Writer
}

var _ runner.Runner = (*dryRunner)(nil)

// Run prints c and returns nil.
func (r *dryRunner) Run(_ context.Context, c runner.Cmd) error {
	r.printf("+ %s\n", c)
	return nil
}

// Output prints c and runs it through inner, unless it is interactive.
func (r *dryRunner) Output(ctx context.Context, c runner.Cmd) ([]byte, error) {
	if c.Interactive {
		r.printf("+ %s\n", c)
		return nil, nil
	}
	r.printf("? %s\n", c)
	return r.inner.Output(ctx, c)
}

func (r *dryRunner) printf(format string, args ...any) {
	// A failed write to the dry-run log is not a command failure.
	_, _ = fmt.Fprintf(r.out, format, args...)
}

// dryFS is the WriteFS of --dry-run. Reads pass through to inner; writes are
// printed as "+ <op> ..." and change nothing.
type dryFS struct {
	inner module.WriteFS
	out   io.Writer
}

var _ module.WriteFS = dryFS{}

// ReadFile calls inner.ReadFile.
func (f dryFS) ReadFile(name string) ([]byte, error) { return f.inner.ReadFile(name) }

// Stat calls inner.Stat.
func (f dryFS) Stat(name string) (fs.FileInfo, error) { return f.inner.Stat(name) }

// Lstat calls inner.Lstat.
func (f dryFS) Lstat(name string) (fs.FileInfo, error) { return f.inner.Lstat(name) }

// WriteFile prints the write and does nothing.
func (f dryFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f.printf("+ write %s (%#o, %d bytes)\n", name, perm.Perm(), len(data))
	return nil
}

// MkdirAll prints the mkdir and does nothing.
func (f dryFS) MkdirAll(path string, perm fs.FileMode) error {
	f.printf("+ mkdir -p %s (%#o)\n", path, perm.Perm())
	return nil
}

// Rename prints the move and does nothing.
func (f dryFS) Rename(oldpath, newpath string) error {
	f.printf("+ mv %s %s\n", oldpath, newpath)
	return nil
}

// Remove prints the removal and does nothing.
func (f dryFS) Remove(name string) error {
	f.printf("+ rm %s\n", name)
	return nil
}

func (f dryFS) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(f.out, format, args...)
}
