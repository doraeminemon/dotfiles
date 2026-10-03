// Package prompt owns every question the CLI asks the user.
//
// Modules depend on the narrow Prompter interface. HuhPrompter is the only
// adapter over charm.land/huh/v2; the rest of the codebase does not import huh.
// FakePrompter serves scripted answers to tests in other packages.
package prompt

import (
	"errors"
	"fmt"
)

// Prompter asks the user for input.
type Prompter interface {
	// Confirm asks a yes/no question. def is the answer on empty input.
	Confirm(title string, def bool) (bool, error)
	// Input asks for one line of text.
	Input(title string, opts InputOpts) (string, error)
	// MultiSelect asks the user to pick zero or more options and returns
	// the Value of each picked option.
	MultiSelect(title string, opts []Option) ([]string, error)
}

// InputOpts configures Prompter.Input.
type InputOpts struct {
	Placeholder string
	// Secret hides the typed text. In accessible mode it needs a TTY reader;
	// without one, HuhPrompter returns *SecretNeedsTTYError.
	Secret bool
	// Validate rejects an answer when it returns a non-nil error. Nil accepts
	// every answer.
	Validate func(string) error
}

// Option is one choice of Prompter.MultiSelect.
type Option struct {
	Label    string
	Value    string
	Selected bool
}

// ErrAborted is returned when the user cancels a prompt (for example ctrl+c).
var ErrAborted = errors.New("prompt: aborted by user")

// SecretNeedsTTYError is returned when a secret input is requested in
// accessible mode but the input reader is not a terminal. Callers fall back to
// an environment variable and report which one is missing.
type SecretNeedsTTYError struct {
	Title string
}

func (e *SecretNeedsTTYError) Error() string {
	return fmt.Sprintf("prompt %q: secret input needs a terminal", e.Title)
}

// InvalidInputError is returned when an answer fails InputOpts.Validate and
// the user cannot be asked again (accessible mode with closed input).
type InvalidInputError struct {
	Title string
	Err   error
}

func (e *InvalidInputError) Error() string {
	return fmt.Sprintf("prompt %q: invalid input: %v", e.Title, e.Err)
}

func (e *InvalidInputError) Unwrap() error { return e.Err }
