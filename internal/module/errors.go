package module

import (
	"fmt"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/platform"
)

// UnknownModuleError reports a module name that is not registered.
type UnknownModuleError struct {
	Name string
}

func (e *UnknownModuleError) Error() string {
	return fmt.Sprintf("unknown module %q", e.Name)
}

// DuplicateModuleError reports two registered modules with the same ID.
type DuplicateModuleError struct {
	ID ID
}

func (e *DuplicateModuleError) Error() string {
	return fmt.Sprintf("module %q registered twice", e.ID)
}

// UnknownDependencyError reports a DependsOn entry that is not registered.
type UnknownDependencyError struct {
	Module ID
	Dep    ID
}

func (e *UnknownDependencyError) Error() string {
	return fmt.Sprintf("module %q depends on unknown module %q", e.Module, e.Dep)
}

// CycleError reports a dependency cycle. Path starts and ends with the same
// ID, for example [a b a].
type CycleError struct {
	Path []ID
}

func (e *CycleError) Error() string {
	parts := make([]string, len(e.Path))
	for i, id := range e.Path {
		parts[i] = string(id)
	}
	return "dependency cycle: " + strings.Join(parts, " -> ")
}

// UnsupportedError reports an explicitly selected module that does not
// support the platform.
type UnsupportedError struct {
	ID       ID
	Platform platform.Platform
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("module %q does not support %s", e.ID, e.Platform)
}
