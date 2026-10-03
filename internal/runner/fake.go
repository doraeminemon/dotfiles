package runner

import (
	"context"
	"sync"
)

// Result is a scripted FakeRunner response.
type Result struct {
	Stdout []byte
	Err    error
}

// FakeRunner records every command and returns scripted results. Module tests
// use it to assert the exact commands a module runs.
type FakeRunner struct {
	// Script maps a full Cmd.String() to its result. A key equal to
	// Cmd.Name is the fallback. Unscripted commands succeed with no output.
	Script map[string]Result
	// Calls holds every command in call order.
	Calls []Cmd

	mu sync.Mutex
}

var _ Runner = (*FakeRunner)(nil)

// Run records c and returns its scripted error.
func (f *FakeRunner) Run(_ context.Context, c Cmd) error {
	return f.record(c).Err
}

// Output records c and returns its scripted stdout and error.
func (f *FakeRunner) Output(_ context.Context, c Cmd) ([]byte, error) {
	r := f.record(c)
	return r.Stdout, r.Err
}

// Commands returns Cmd.String() of each recorded call, in order.
func (f *FakeRunner) Commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		out[i] = c.String()
	}
	return out
}

func (f *FakeRunner) record(c Cmd) Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, c)
	if r, ok := f.Script[c.String()]; ok {
		return r
	}
	return f.Script[c.Name]
}
