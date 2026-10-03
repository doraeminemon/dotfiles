// Package module defines the installer Module interface, the Env that modules
// run in, and the Registry that orders modules by their dependencies.
//
// Concrete modules live in internal/modules. cmd/dot builds the Env at the
// program edge, parses --only values with Registry.ParseID, and runs the
// modules that Registry.Plan returns, in order.
package module

import (
	"context"

	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// ID names a module. An ID from user input is valid only after
// Registry.ParseID accepted it.
type ID string

// Status is the result of Module.Check.
type Status int

// Module statuses.
const (
	// StatusMissing means Apply has work to do.
	StatusMissing Status = iota
	// StatusInstalled means the module is already in place.
	StatusInstalled
	// StatusUnsupported means the module does not apply to this platform.
	StatusUnsupported
)

// String returns a lowercase name for s, for example "installed".
func (s Status) String() string {
	switch s {
	case StatusMissing:
		return "missing"
	case StatusInstalled:
		return "installed"
	case StatusUnsupported:
		return "unsupported"
	}
	return "unknown"
}

// Env is everything a module may touch. All process execution goes through
// Run, all user input through Ask, and all file writes through FS.
type Env struct {
	Platform platform.Platform
	Run      runner.Runner
	Ask      prompt.Prompter
	FS       WriteFS
	// RepoDir is the checkout of this repo that modules read data from.
	RepoDir string
	// Home is the user's home directory.
	Home string
	// Yes answers every confirmation with yes, for non-interactive runs.
	Yes bool
}

// Confirm returns true when env.Yes is set. Otherwise it asks the user.
func (env Env) Confirm(title string, def bool) (bool, error) {
	if env.Yes {
		return true, nil
	}
	return env.Ask.Confirm(title, def)
}

// Module is one installer step.
type Module interface {
	ID() ID
	// Summary is a one-line description for lists and prompts.
	Summary() string
	// DependsOn lists the modules that must run before this one. A
	// dependency that does not support the platform is skipped.
	DependsOn() []ID
	Supports(p platform.Platform) bool
	// Check reports whether Apply has work to do. It changes nothing.
	Check(ctx context.Context, env Env) (Status, error)
	Apply(ctx context.Context, env Env) error
}
