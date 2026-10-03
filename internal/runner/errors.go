package runner

import (
	"fmt"
	"strings"
)

// CommandError reports a command that exited with a non-zero status.
type CommandError struct {
	Cmd      Cmd
	ExitCode int
	// Stderr holds the last StderrLimit bytes of the command's stderr.
	// It is empty for interactive commands, whose stderr goes to the terminal.
	Stderr string
}

func (e *CommandError) Error() string {
	msg := fmt.Sprintf("%s: exit status %d", e.Cmd, e.ExitCode)
	if s := strings.TrimSpace(e.Stderr); s != "" {
		msg += ": " + s
	}
	return msg
}

// NotFoundError reports a program that is not in PATH.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s: command not found", e.Name)
}
