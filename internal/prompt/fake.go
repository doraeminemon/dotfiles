package prompt

import (
	"context"
	"fmt"
)

var _ Prompter = (*FakePrompter)(nil)

// Kind names a Prompter method, for ScriptExhaustedError.
type Kind string

const (
	KindConfirm     Kind = "confirm"
	KindInput       Kind = "input"
	KindMultiSelect Kind = "multiselect"
)

// ScriptExhaustedError is returned by FakePrompter when a test asks more
// questions of one Kind than it scripted answers for.
type ScriptExhaustedError struct {
	Kind Kind
}

func (e *ScriptExhaustedError) Error() string {
	return fmt.Sprintf("prompt: fake has no scripted %s answer left", e.Kind)
}

// FakePrompter returns scripted answers in order, one queue per method, and
// records every title it is asked in Asked. Input runs InputOpts.Validate on
// the scripted answer and returns its error unchanged.
type FakePrompter struct {
	Confirms   []bool
	Inputs     []string
	Selections [][]string
	Asked      []string
}

// Confirm implements Prompter.
func (f *FakePrompter) Confirm(title string, _ bool) (bool, error) {
	f.Asked = append(f.Asked, title)
	if len(f.Confirms) == 0 {
		return false, &ScriptExhaustedError{Kind: KindConfirm}
	}
	v := f.Confirms[0]
	f.Confirms = f.Confirms[1:]
	return v, nil
}

// Input implements Prompter.
func (f *FakePrompter) Input(title string, opts InputOpts) (string, error) {
	f.Asked = append(f.Asked, title)
	if len(f.Inputs) == 0 {
		return "", &ScriptExhaustedError{Kind: KindInput}
	}
	v := f.Inputs[0]
	f.Inputs = f.Inputs[1:]
	if opts.Validate != nil {
		if err := opts.Validate(v); err != nil {
			return "", err
		}
	}
	return v, nil
}

// MultiSelect implements Prompter.
func (f *FakePrompter) MultiSelect(title string, _ []Option) ([]string, error) {
	f.Asked = append(f.Asked, title)
	if len(f.Selections) == 0 {
		return nil, &ScriptExhaustedError{Kind: KindMultiSelect}
	}
	v := f.Selections[0]
	f.Selections = f.Selections[1:]
	return v, nil
}

// Spinner records title and runs fn, matching HuhPrompter.Spinner.
func (f *FakePrompter) Spinner(ctx context.Context, title string, fn func(context.Context) error) error {
	f.Asked = append(f.Asked, title)
	return fn(ctx)
}
