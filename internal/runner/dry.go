package runner

import (
	"context"
	"fmt"
	"io"
)

// DryRunner prints each command as "+ <cmd>" and runs nothing.
type DryRunner struct {
	// Out receives one line per command. Nil prints nothing.
	Out io.Writer
}

var _ Runner = (*DryRunner)(nil)

// Run prints c and returns nil.
func (d *DryRunner) Run(_ context.Context, c Cmd) error {
	d.print(c)
	return nil
}

// Output prints c and returns no output and no error.
func (d *DryRunner) Output(_ context.Context, c Cmd) ([]byte, error) {
	d.print(c)
	return nil, nil
}

func (d *DryRunner) print(c Cmd) {
	if d.Out == nil {
		return
	}
	// A failed write to the dry-run log is not a command failure.
	_, _ = fmt.Fprintf(d.Out, "+ %s\n", c)
}
