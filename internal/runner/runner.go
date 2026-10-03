// Package runner is the single place where dot executes external processes.
//
// Modules depend on the Runner interface. cmd/dot injects ExecRunner for real
// runs or DryRunner for --dry-run, and tests inject FakeRunner.
package runner

import (
	"context"
	"io"
	"strings"
)

// Cmd describes one process invocation.
type Cmd struct {
	// Name is the program, resolved through PATH.
	Name string
	// Args are the arguments after Name.
	Args []string
	// Env entries ("KEY=value") are appended to the parent environment.
	Env []string
	// Dir is the working directory. Empty means the current directory.
	Dir string
	// Stdin feeds the process. Nil means no input, or the terminal when
	// Interactive is true.
	Stdin io.Reader
	// Interactive connects the process to the real terminal. Use it for
	// commands that prompt the user (sudo, chsh, ssh-keygen, gh auth).
	Interactive bool
}

// Runner executes commands.
type Runner interface {
	// Run executes c and waits for it to finish.
	Run(ctx context.Context, c Cmd) error
	// Output executes c and returns its stdout. Stderr is captured into the
	// returned *CommandError on a non-zero exit.
	Output(ctx context.Context, c Cmd) ([]byte, error)
}

// String renders c as a shell-quoted command line: env assignments, then the
// name and arguments. Dir, Stdin, and Interactive are not rendered.
func (c Cmd) String() string {
	parts := make([]string, 0, len(c.Env)+1+len(c.Args))
	for _, e := range c.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			parts = append(parts, quote(e))
			continue
		}
		parts = append(parts, k+"="+quote(v))
	}
	parts = append(parts, quote(c.Name))
	for _, a := range c.Args {
		parts = append(parts, quote(a))
	}
	return strings.Join(parts, " ")
}

// quote returns s unchanged when it has only shell-safe characters, and
// single-quoted otherwise.
func quote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, unsafeRune) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func unsafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("-_./:=@%+,", r)
}
