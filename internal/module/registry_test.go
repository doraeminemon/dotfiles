package module_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
)

var (
	darwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	linux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
)

type fake struct {
	id   module.ID
	deps []module.ID
	only *platform.OS
}

func (f fake) ID() module.ID          { return f.id }
func (f fake) Summary() string        { return "fake " + string(f.id) }
func (f fake) DependsOn() []module.ID { return f.deps }
func (f fake) Supports(p platform.Platform) bool {
	return f.only == nil || *f.only == p.OS
}

func (f fake) Check(context.Context, module.Env) (module.Status, error) {
	return module.StatusMissing, nil
}
func (f fake) Apply(context.Context, module.Env) error { return nil }

func mod(id module.ID, deps ...module.ID) fake { return fake{id: id, deps: deps} }

func linuxOnly(id module.ID, deps ...module.ID) fake {
	os := platform.Linux
	return fake{id: id, deps: deps, only: &os}
}

func mustRegistry(t *testing.T, mods ...module.Module) *module.Registry {
	t.Helper()
	r, err := module.NewRegistry(mods...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

func ids(mods []module.Module) []module.ID {
	out := make([]module.ID, len(mods))
	for i, m := range mods {
		out[i] = m.ID()
	}
	return out
}

// installer mirrors the real module graph shape.
func installer(t *testing.T) *module.Registry {
	t.Helper()
	return mustRegistry(t,
		mod("repo", "apt"),
		linuxOnly("apt"),
		mod("brew", "repo", "apt"),
		mod("chezmoi", "repo", "brew"),
		mod("fish", "brew"),
		mod("ssh"),
	)
}

func TestPlanOrder(t *testing.T) {
	r := installer(t)
	tests := []struct {
		name     string
		selected []module.ID
		p        platform.Platform
		want     []module.ID
	}{
		{"all on linux", nil, linux, []module.ID{"apt", "repo", "brew", "chezmoi", "fish", "ssh"}},
		{"all on darwin skips apt", nil, darwin, []module.ID{"repo", "brew", "chezmoi", "fish", "ssh"}},
		{"adds transitive deps", []module.ID{"chezmoi"}, linux, []module.ID{"apt", "repo", "brew", "chezmoi"}},
		{"skips unsupported dep", []module.ID{"fish"}, darwin, []module.ID{"repo", "brew", "fish"}},
		{"selection order does not matter", []module.ID{"ssh", "fish"}, darwin, []module.ID{"repo", "brew", "fish", "ssh"}},
		{"duplicates collapse", []module.ID{"ssh", "ssh"}, darwin, []module.ID{"ssh"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Plan(tt.selected, tt.p)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if !slices.Equal(ids(got), tt.want) {
				t.Errorf("Plan = %v, want %v", ids(got), tt.want)
			}
		})
	}
}

func TestPlanTiesKeepRegistrationOrder(t *testing.T) {
	// d registers first but depends on c, so c moves ahead of it; a and b
	// have no order between them and keep registration order.
	r := mustRegistry(t, mod("d", "c"), mod("b"), mod("a"), mod("c"))
	got, err := r.Plan(nil, darwin)
	if err != nil {
		t.Fatal(err)
	}
	if want := []module.ID{"b", "a", "c", "d"}; !slices.Equal(ids(got), want) {
		t.Errorf("Plan = %v, want %v", ids(got), want)
	}
}

func TestPlanCycle(t *testing.T) {
	r := mustRegistry(t, mod("a", "b"), mod("b", "c"), mod("c", "a"), mod("x"))
	_, err := r.Plan([]module.ID{"a"}, darwin)
	var ce *module.CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CycleError", err)
	}
	if want := []module.ID{"a", "b", "c", "a"}; !slices.Equal(ce.Path, want) {
		t.Errorf("Path = %v, want %v", ce.Path, want)
	}

	if _, err := r.Plan([]module.ID{"x"}, darwin); err != nil {
		t.Errorf("Plan(x) = %v, want nil: the cycle is not in the plan", err)
	}
}

func TestPlanSelfCycle(t *testing.T) {
	r := mustRegistry(t, mod("a", "a"))
	_, err := r.Plan(nil, darwin)
	var ce *module.CycleError
	if !errors.As(err, &ce) || !slices.Equal(ce.Path, []module.ID{"a", "a"}) {
		t.Fatalf("err = %v, want cycle a -> a", err)
	}
}

func TestPlanUnsupportedSelection(t *testing.T) {
	r := installer(t)
	_, err := r.Plan([]module.ID{"apt"}, darwin)
	var ue *module.UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want *UnsupportedError", err)
	}
	if ue.ID != "apt" || ue.Platform != darwin {
		t.Errorf("got %+v", ue)
	}
}

func TestPlanUnknownID(t *testing.T) {
	r := installer(t)
	_, err := r.Plan([]module.ID{"nope"}, darwin)
	var ue *module.UnknownModuleError
	if !errors.As(err, &ue) || ue.Name != "nope" {
		t.Fatalf("err = %v, want *UnknownModuleError{nope}", err)
	}
}

func TestParseID(t *testing.T) {
	r := installer(t)
	id, err := r.ParseID("fish")
	if err != nil || id != "fish" {
		t.Fatalf("ParseID(fish) = %q, %v", id, err)
	}
	_, err = r.ParseID("Fish")
	var ue *module.UnknownModuleError
	if !errors.As(err, &ue) || ue.Name != "Fish" {
		t.Fatalf("ParseID(Fish) err = %v, want *UnknownModuleError", err)
	}
}

func TestNewRegistryErrors(t *testing.T) {
	_, err := module.NewRegistry(mod("a"), mod("b"), mod("a"))
	var de *module.DuplicateModuleError
	if !errors.As(err, &de) || de.ID != "a" {
		t.Errorf("duplicate: err = %v, want *DuplicateModuleError{a}", err)
	}

	_, err = module.NewRegistry(mod("a", "ghost"))
	var ude *module.UnknownDependencyError
	if !errors.As(err, &ude) || ude.Module != "a" || ude.Dep != "ghost" {
		t.Errorf("unknown dep: err = %v, want *UnknownDependencyError{a ghost}", err)
	}
}

func TestAllKeepsRegistrationOrderAndIsACopy(t *testing.T) {
	r := installer(t)
	all := r.All()
	want := []module.ID{"repo", "apt", "brew", "chezmoi", "fish", "ssh"}
	if !slices.Equal(ids(all), want) {
		t.Fatalf("All = %v, want %v", ids(all), want)
	}
	all[0] = mod("mutated")
	if r.All()[0].ID() != "repo" {
		t.Error("All returned the registry's own slice")
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[module.Status]string{
		module.StatusMissing:     "missing",
		module.StatusInstalled:   "installed",
		module.StatusUnsupported: "unsupported",
	} {
		if s.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(s), s.String(), want)
		}
	}
}

func TestEnvConfirm(t *testing.T) {
	ask := &prompt.FakePrompter{Confirms: []bool{false}}
	env := module.Env{Ask: ask, Yes: true}
	if ok, err := env.Confirm("go?", false); err != nil || !ok {
		t.Fatalf("Yes: Confirm = %v, %v; want true", ok, err)
	}
	if len(ask.Asked) != 0 {
		t.Fatalf("Yes: asked %v, want no prompt", ask.Asked)
	}
	env.Yes = false
	if ok, err := env.Confirm("go?", true); err != nil || ok {
		t.Fatalf("Confirm = %v, %v; want scripted false", ok, err)
	}
	if !slices.Equal(ask.Asked, []string{"go?"}) {
		t.Errorf("Asked = %v", ask.Asked)
	}
}
