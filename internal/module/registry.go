package module

import (
	"slices"

	"github.com/doraeminemon/dotfiles/internal/platform"
)

// Registry is the fixed set of modules, in registration order.
type Registry struct {
	mods  []Module
	index map[ID]int
}

// NewRegistry registers mods in order. It returns *DuplicateModuleError when
// two modules share an ID, and *UnknownDependencyError when a DependsOn entry
// names a module that is not in mods.
func NewRegistry(mods ...Module) (*Registry, error) {
	r := &Registry{mods: slices.Clone(mods), index: make(map[ID]int, len(mods))}
	for i, m := range r.mods {
		if _, ok := r.index[m.ID()]; ok {
			return nil, &DuplicateModuleError{ID: m.ID()}
		}
		r.index[m.ID()] = i
	}
	for _, m := range r.mods {
		for _, d := range m.DependsOn() {
			if _, ok := r.index[d]; !ok {
				return nil, &UnknownDependencyError{Module: m.ID(), Dep: d}
			}
		}
	}
	return r, nil
}

// ParseID returns the ID of the registered module named name, or
// *UnknownModuleError.
func (r *Registry) ParseID(name string) (ID, error) {
	if _, ok := r.index[ID(name)]; !ok {
		return "", &UnknownModuleError{Name: name}
	}
	return ID(name), nil
}

// All returns every module in registration order.
func (r *Registry) All() []Module {
	return slices.Clone(r.mods)
}

// Plan returns the modules to run on p, in dependency order.
//
// It adds the transitive dependencies of selected. A dependency that does not
// support p is skipped, with its own dependencies. A selected module that does
// not support p is an *UnsupportedError. An empty selected means every module
// that supports p. Modules with no order between them keep registration
// order. Plan returns *UnknownModuleError for an unregistered ID and
// *CycleError for a dependency cycle.
func (r *Registry) Plan(selected []ID, p platform.Platform) ([]Module, error) {
	roots, err := r.roots(selected, p)
	if err != nil {
		return nil, err
	}

	// in holds the registry index of every module in the plan.
	in := make(map[int]bool)
	var stack []int
	onStack := make(map[int]bool)
	var visit func(i int) error
	visit = func(i int) error {
		if onStack[i] {
			start := slices.Index(stack, i)
			path := make([]ID, 0, len(stack)-start+1)
			for _, j := range stack[start:] {
				path = append(path, r.mods[j].ID())
			}
			return &CycleError{Path: append(path, r.mods[i].ID())}
		}
		if in[i] {
			return nil
		}
		stack = append(stack, i)
		onStack[i] = true
		for _, j := range r.supportedDeps(i, p) {
			if err := visit(j); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		onStack[i] = false
		in[i] = true
		return nil
	}
	for _, i := range roots {
		if err := visit(i); err != nil {
			return nil, err
		}
	}

	// Kahn's algorithm, always taking the ready module that registered first.
	pending := make(map[int]int, len(in))
	dependents := make(map[int][]int, len(in))
	for i := range in {
		deps := r.supportedDeps(i, p)
		pending[i] = len(deps)
		for _, j := range deps {
			dependents[j] = append(dependents[j], i)
		}
	}
	plan := make([]Module, 0, len(in))
	for len(plan) < len(in) {
		next := -1
		for i := range r.mods {
			if in[i] && pending[i] == 0 {
				next = i
				break
			}
		}
		// The DFS above rejected cycles, so a ready module always exists.
		pending[next] = -1
		plan = append(plan, r.mods[next])
		for _, d := range dependents[next] {
			pending[d]--
		}
	}
	return plan, nil
}

// roots resolves selected to registry indexes, without duplicates.
func (r *Registry) roots(selected []ID, p platform.Platform) ([]int, error) {
	if len(selected) == 0 {
		var all []int
		for i, m := range r.mods {
			if m.Supports(p) {
				all = append(all, i)
			}
		}
		return all, nil
	}
	roots := make([]int, 0, len(selected))
	for _, id := range selected {
		i, ok := r.index[id]
		if !ok {
			return nil, &UnknownModuleError{Name: string(id)}
		}
		if !r.mods[i].Supports(p) {
			return nil, &UnsupportedError{ID: id, Platform: p}
		}
		if !slices.Contains(roots, i) {
			roots = append(roots, i)
		}
	}
	return roots, nil
}

// supportedDeps returns the registry indexes of the dependencies of module i
// that support p, without duplicates.
func (r *Registry) supportedDeps(i int, p platform.Platform) []int {
	var deps []int
	for _, d := range r.mods[i].DependsOn() {
		j := r.index[d] // NewRegistry checked that every dependency exists.
		if r.mods[j].Supports(p) && !slices.Contains(deps, j) {
			deps = append(deps, j)
		}
	}
	return deps
}
