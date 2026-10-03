package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/project"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

var darwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}

// fakeModule is a scripted module. Check and Apply append "check <id>" and
// "apply <id>" to *log.
type fakeModule struct {
	id       module.ID
	deps     []module.ID
	darwin   bool
	status   module.Status
	checkErr error
	applyErr error
	// cmd, when set, is run by Apply through env.Run.
	cmd *runner.Cmd
	log *[]string
}

func (m *fakeModule) ID() module.ID                   { return m.id }
func (m *fakeModule) Summary() string                 { return "summary of " + string(m.id) }
func (m *fakeModule) DependsOn() []module.ID          { return m.deps }
func (m *fakeModule) Supports(platform.Platform) bool { return m.darwin }

func (m *fakeModule) Check(context.Context, module.Env) (module.Status, error) {
	*m.log = append(*m.log, "check "+string(m.id))
	return m.status, m.checkErr
}

func (m *fakeModule) Apply(ctx context.Context, env module.Env) error {
	*m.log = append(*m.log, "apply "+string(m.id))
	if m.cmd != nil {
		if err := env.Run.Run(ctx, *m.cmd); err != nil {
			return err
		}
	}
	return m.applyErr
}

type fixture struct {
	d      deps
	log    *[]string
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	run    *runner.FakeRunner
	ask    *prompt.FakePrompter
}

// newFixture registers a (installed), b -> a, c -> b, and linux-only l.
func newFixture(mods ...func(log *[]string) *fakeModule) fixture {
	log := &[]string{}
	if len(mods) == 0 {
		mods = []func(*[]string) *fakeModule{
			func(l *[]string) *fakeModule {
				return &fakeModule{id: "a", darwin: true, status: module.StatusInstalled, log: l}
			},
			func(l *[]string) *fakeModule {
				return &fakeModule{id: "b", deps: []module.ID{"a"}, darwin: true, log: l}
			},
			func(l *[]string) *fakeModule {
				return &fakeModule{id: "c", deps: []module.ID{"b"}, darwin: true, log: l}
			},
			func(l *[]string) *fakeModule { return &fakeModule{id: "l", log: l} },
		}
	}
	ms := make([]module.Module, len(mods))
	for i, f := range mods {
		ms[i] = f(log)
	}
	f := fixture{
		log:    log,
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		run:    &runner.FakeRunner{},
		ask:    &prompt.FakePrompter{},
	}
	f.d = deps{
		stdout:   f.stdout,
		stderr:   f.stderr,
		platform: darwin,
		run:      f.run,
		query:    f.run,
		ask:      f.ask,
		fs:       memProjectFS{&module.MemFS{}},
		cwd:      "/work",
		home:     "/home/u",
		modules:  ms,
	}
	return f
}

// memProjectFS adds a ReadDir that lists nothing to MemFS.
type memProjectFS struct{ *module.MemFS }

func (memProjectFS) ReadDir(string) ([]os.DirEntry, error) { return nil, nil }

var _ project.FS = memProjectFS{}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		code       int
		wantStdout string
		wantStderr string
	}{
		{"no args", nil, 2, "", "Usage:"},
		{"help", []string{"help"}, 0, "Usage:", ""},
		{"version", []string{"version"}, 0, "dev", ""},
		{"unknown command", []string{"frob"}, 2, "", `unknown command "frob"`},
		{"bad flag", []string{"install", "--nope"}, 2, "", "flag provided but not defined"},
		{"install -h", []string{"install", "-h"}, 0, "", "-dry-run"},
		{"unknown only", []string{"install", "--only", "a,zzz", "--yes"}, 2, "", "valid modules: a, b, c, l"},
		{"unsupported only", []string{"install", "--only", "l", "--yes"}, 2, "", `module "l" does not support`},
		{"skills without apply", []string{"skills"}, 2, "", "usage: dot skills apply"},
		{"agents unknown template", []string{"agents", "apply", "x"}, 2, "", `unknown agents template "x"`},
		{"list", []string{"list"}, 0, "summary of a", ""},
		{"doctor unhealthy", []string{"doctor"}, 1, "missing", "not installed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			if code := run(t.Context(), tt.args, f.d); code != tt.code {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, f.stdout, f.stderr)
			}
			if !strings.Contains(f.stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", f.stdout, tt.wantStdout)
			}
			if !strings.Contains(f.stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", f.stderr, tt.wantStderr)
			}
		})
	}
}

func TestInstallOnlyOrdersDependencies(t *testing.T) {
	f := newFixture()
	if code := run(t.Context(), []string{"install", "--only", "c", "--yes"}, f.d); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, f.stderr)
	}
	want := []string{"check a", "check b", "apply b", "check c", "apply c"}
	if !slices.Equal(*f.log, want) {
		t.Errorf("calls = %v, want %v", *f.log, want)
	}
	out := f.stdout.String()
	for _, s := range []string{"Plan: a → b → c", "==> a — summary of a", "already installed", "==> c — summary of c"} {
		if !strings.Contains(out, s) {
			t.Errorf("stdout = %q, want it to contain %q", out, s)
		}
	}
}

func TestInstallStopsAtFirstError(t *testing.T) {
	cmdErr := &runner.CommandError{Cmd: runner.Cmd{Name: "brew", Args: []string{"install", "x"}}, ExitCode: 3, Stderr: "line1\nboom\n"}
	f := newFixture(
		func(l *[]string) *fakeModule { return &fakeModule{id: "a", darwin: true, applyErr: cmdErr, log: l} },
		func(l *[]string) *fakeModule { return &fakeModule{id: "b", darwin: true, log: l} },
	)
	if code := run(t.Context(), []string{"install", "--yes"}, f.d); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if want := []string{"check a", "apply a"}; !slices.Equal(*f.log, want) {
		t.Errorf("calls = %v, want %v", *f.log, want)
	}
	for _, s := range []string{"command failed: brew install x (exit status 3)", "boom"} {
		if !strings.Contains(f.stderr.String(), s) {
			t.Errorf("stderr = %q, want it to contain %q", f.stderr, s)
		}
	}
}

func TestInstallErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		want string
	}{
		{"not found", &runner.NotFoundError{Name: "brew"}, 1, "install brew first"},
		{"cycle", &module.CycleError{Path: []module.ID{"a", "b", "a"}}, 1, "a -> b -> a"},
		{"aborted", prompt.ErrAborted, 130, "aborted"},
		{"exists", &project.ExistsError{Path: "/x"}, 1, "use --force"},
		{"other", errors.New("kaput"), 1, "kaput"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(func(l *[]string) *fakeModule {
				return &fakeModule{id: "a", darwin: true, applyErr: tt.err, log: l}
			})
			if code := run(t.Context(), []string{"install", "--yes"}, f.d); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if !strings.Contains(f.stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", f.stderr, tt.want)
			}
		})
	}
}

func TestInstallPicker(t *testing.T) {
	f := newFixture()
	f.d.stdinTTY = true
	f.ask.Selections = [][]string{{"b"}}
	if code := run(t.Context(), []string{"install"}, f.d); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, f.stderr)
	}
	// The picker checks a, b, c for their labels; then the plan for b runs.
	want := []string{"check a", "check b", "check c", "check a", "check b", "apply b"}
	if !slices.Equal(*f.log, want) {
		t.Errorf("calls = %v, want %v", *f.log, want)
	}
}

func TestInstallPickerAborted(t *testing.T) {
	f := newFixture()
	f.d.stdinTTY = true
	f.d.ask = &abortPrompter{}
	if code := run(t.Context(), []string{"install"}, f.d); code != 130 {
		t.Errorf("exit code = %d, want 130", code)
	}
}

type abortPrompter struct{ prompt.FakePrompter }

func (abortPrompter) MultiSelect(string, []prompt.Option) ([]string, error) {
	return nil, prompt.ErrAborted
}

func TestInstallDryRunNeverRuns(t *testing.T) {
	f := newFixture(func(l *[]string) *fakeModule {
		return &fakeModule{id: "a", darwin: true, cmd: &runner.Cmd{Name: "brew", Args: []string{"install", "fish"}}, log: l}
	})
	if code := run(t.Context(), []string{"install", "--dry-run", "--yes"}, f.d); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, f.stderr)
	}
	if len(f.run.Calls) != 0 {
		t.Errorf("inner runner got %v, want no calls", f.run.Commands())
	}
	if !strings.Contains(f.stdout.String(), "+ brew install fish") {
		t.Errorf("stdout = %q, want the printed command", f.stdout)
	}
}

func TestInstallDryRunContinuesAfterErrors(t *testing.T) {
	cmdErr := &runner.CommandError{Cmd: runner.Cmd{Name: "brew", Args: []string{"install", "x"}}, ExitCode: 3, Stderr: "boom\n"}
	f := newFixture(
		func(l *[]string) *fakeModule {
			return &fakeModule{id: "a", darwin: true, checkErr: errors.New("no list"), log: l}
		},
		func(l *[]string) *fakeModule { return &fakeModule{id: "b", darwin: true, applyErr: cmdErr, log: l} },
		func(l *[]string) *fakeModule { return &fakeModule{id: "c", darwin: true, log: l} },
	)
	if code := run(t.Context(), []string{"install", "--dry-run", "--yes"}, f.d); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if want := []string{"check a", "check b", "apply b", "check c", "apply c"}; !slices.Equal(*f.log, want) {
		t.Errorf("calls = %v, want %v", *f.log, want)
	}
	for _, s := range []string{"! a: check: no list", "! b: command failed: brew install x (exit status 3)", "boom"} {
		if !strings.Contains(f.stderr.String(), s) {
			t.Errorf("stderr = %q, want it to contain %q", f.stderr, s)
		}
	}
	for _, id := range []string{"a", "b", "c"} {
		if !strings.Contains(f.stdout.String(), "==> "+id) {
			t.Errorf("stdout = %q, want a header for %s", f.stdout, id)
		}
	}
	if want := "Dry run finished: 2 of 3 steps failed: a, b"; !strings.Contains(f.stdout.String(), want) {
		t.Errorf("stdout = %q, want %q", f.stdout, want)
	}
}

func TestInstallDryRunAllOK(t *testing.T) {
	f := newFixture()
	if code := run(t.Context(), []string{"install", "--dry-run", "--yes"}, f.d); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, f.stderr)
	}
	if want := "Dry run finished: all 3 steps OK"; !strings.Contains(f.stdout.String(), want) {
		t.Errorf("stdout = %q, want %q", f.stdout, want)
	}
}

func TestInstallDryRunAbortStops(t *testing.T) {
	f := newFixture(
		func(l *[]string) *fakeModule {
			return &fakeModule{id: "a", darwin: true, applyErr: prompt.ErrAborted, log: l}
		},
		func(l *[]string) *fakeModule { return &fakeModule{id: "b", darwin: true, log: l} },
	)
	if code := run(t.Context(), []string{"install", "--dry-run", "--yes"}, f.d); code != 130 {
		t.Fatalf("exit code = %d, want 130", code)
	}
	if want := []string{"check a", "apply a"}; !slices.Equal(*f.log, want) {
		t.Errorf("calls = %v, want %v", *f.log, want)
	}
}

func TestDryRunner(t *testing.T) {
	inner := &runner.FakeRunner{Script: map[string]runner.Result{"brew list": {Stdout: []byte("fish\n")}}}
	var out bytes.Buffer
	r := &dryRunner{inner: inner, out: &out}
	ctx := t.Context()

	if err := r.Run(ctx, runner.Cmd{Name: "brew", Args: []string{"install", "fish"}}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Output(ctx, runner.Cmd{Name: "brew", Args: []string{"list"}})
	if err != nil || string(got) != "fish\n" {
		t.Errorf("Output = %q, %v; want the inner stdout", got, err)
	}
	if _, err := r.Output(ctx, runner.Cmd{Name: "gh", Args: []string{"auth", "login"}, Interactive: true}); err != nil {
		t.Fatal(err)
	}

	if want := []string{"brew list"}; !slices.Equal(inner.Commands(), want) {
		t.Errorf("inner calls = %v, want %v", inner.Commands(), want)
	}
	want := "+ brew install fish\n? brew list\n+ gh auth login\n"
	if out.String() != want {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
}

func TestDryFSNeverWrites(t *testing.T) {
	inner := &module.MemFS{}
	if err := inner.MkdirAll("/h", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := inner.WriteFile("/h/a", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	f := dryFS{inner: inner, out: &out}

	if data, err := f.ReadFile("/h/a"); err != nil || string(data) != "x" {
		t.Errorf("ReadFile = %q, %v", data, err)
	}
	if _, err := f.Stat("/h/a"); err != nil {
		t.Errorf("Stat: %v", err)
	}
	if _, err := f.Lstat("/h/a"); err != nil {
		t.Errorf("Lstat: %v", err)
	}
	for _, err := range []error{
		f.WriteFile("/h/b", []byte("yy"), 0o600),
		f.MkdirAll("/h/d", 0o700),
		f.Rename("/h/a", "/h/c"),
		f.Remove("/h/a"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, p := range []string{"/h/b", "/h/d", "/h/c"} {
		if _, err := inner.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists after a dry write (err %v)", p, err)
		}
	}
	if data, err := inner.ReadFile("/h/a"); err != nil || string(data) != "x" {
		t.Errorf("/h/a changed: %q, %v", data, err)
	}
	want := "+ write /h/b (0600, 2 bytes)\n+ mkdir -p /h/d (0700)\n+ mv /h/a /h/c\n+ rm /h/a\n"
	if out.String() != want {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
}

func TestFindRepoDir(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "checkout")
	deep := filepath.Join(repo, "cmd", "dot")
	other := filepath.Join(root, "other", "sub")
	for _, dir := range []string{deep, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(repo, "go.mod"), "// comment\nmodule github.com/doraeminemon/dotfiles\n\ngo 1.26.0\n")
	write(filepath.Join(root, "other", "go.mod"), "module example.com/other\n")

	home := "/home/u"
	fallback := filepath.Join(home, "Projects", "dotfiles")
	tests := []struct {
		cwd, want string
	}{
		{repo, repo},
		{deep, repo},
		{other, fallback},
		{root, fallback},
	}
	for _, tt := range tests {
		if got := findRepoDir(module.OSFS{}, tt.cwd, home); got != tt.want {
			t.Errorf("findRepoDir(%s) = %s, want %s", tt.cwd, got, tt.want)
		}
	}
}

func TestGoModPath(t *testing.T) {
	tests := map[string]string{
		"module a/b\n":                "a/b",
		"module \"a/b\" // x\n":       "a/b",
		"modules x\nmodule a/b\n":     "a/b",
		"go 1.26\n":                   "",
		"  module\ta/b\nmodule c/d\n": "a/b",
	}
	for in, want := range tests {
		if got := goModPath([]byte(in)); got != want {
			t.Errorf("goModPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrependPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", strings.Join([]string{"/usr/bin", "/opt/homebrew/bin", "/bin"}, sep))
	if err := prependPath("/opt/homebrew/bin", "/opt/homebrew/sbin"); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/bin", "/bin"}, sep)
	if got := os.Getenv("PATH"); got != want {
		t.Errorf("PATH = %q, want %q", got, want)
	}
}

func TestListFlattensErrors(t *testing.T) {
	f := newFixture(func(l *[]string) *fakeModule {
		return &fakeModule{id: "a", darwin: true, checkErr: errors.New("first\nsecond " + strings.Repeat("x", 200)), log: l}
	})
	if code := run(t.Context(), []string{"list"}, f.d); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(f.stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want header + 1 row:\n%s", len(lines), f.stdout)
	}
	if !strings.Contains(lines[1], "error: first second x") || !strings.Contains(lines[1], "…") {
		t.Errorf("row = %q, want a flattened, cut error", lines[1])
	}
}

func TestAllModulesRegister(t *testing.T) {
	reg, err := module.NewRegistry(allModules()...)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reg.All()); got != 11 {
		t.Errorf("registered %d modules, want 11", got)
	}
	if _, err := reg.Plan(nil, darwin); err != nil {
		t.Errorf("Plan(darwin): %v", err)
	}
}
