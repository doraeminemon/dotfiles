package modules

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/pkglist"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

var (
	brewTestDarwinARM = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	brewTestDarwinAMD = platform.Platform{OS: platform.Darwin, Arch: platform.AMD64}
	brewTestLinux     = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
)

const (
	brewTestRepo      = "/repo"
	brewTestFormulae  = "# core\ngit\nfish\nowner/tap/rtk\n"
	brewTestCasks     = "claude-code\nminiforge\n"
	brewTestInstaller = "NONINTERACTIVE=1 /bin/bash -c 'curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | /bin/bash'"

	brewTestDarwinBrew = "/opt/homebrew/bin/brew"
	brewTestIntelBrew  = "/usr/local/bin/brew"
	brewTestLinuxBrew  = "/home/linuxbrew/.linuxbrew/bin/brew"
)

// brewTestPath records the directories that NewBrew's prependPath receives.
type brewTestPath struct {
	calls [][]string
	err   error
}

func (r *brewTestPath) prepend(dirs ...string) error {
	r.calls = append(r.calls, dirs)
	return r.err
}

type brewTestCase struct {
	plat        platform.Platform
	brewPresent bool
	// casks false leaves brew-cask-darwin.txt out of the repo.
	casks  bool
	script map[string]runner.Result
}

func newBrewTestEnv(t *testing.T, tc brewTestCase) (module.Env, *runner.FakeRunner) {
	t.Helper()
	mfs := &module.MemFS{}
	pkgs := filepath.Join(brewTestRepo, "packages")
	brewTestMust(t, mfs.MkdirAll(pkgs, 0o755))
	brewTestMust(t, mfs.WriteFile(filepath.Join(pkgs, "brew.txt"), []byte(brewTestFormulae), 0o644))
	if tc.casks {
		brewTestMust(t, mfs.WriteFile(filepath.Join(pkgs, "brew-cask-darwin.txt"), []byte(brewTestCasks), 0o644))
	}
	if tc.brewPresent {
		prefix, ok := brewPrefix(tc.plat)
		if !ok {
			t.Fatalf("no brew prefix for %v", tc.plat)
		}
		brewTestMust(t, mfs.MkdirAll(prefix+"/bin", 0o755))
		brewTestMust(t, mfs.WriteFile(prefix+"/bin/brew", nil, 0o755))
	}
	fr := &runner.FakeRunner{Script: tc.script}
	return module.Env{
		Platform: tc.plat,
		Run:      fr,
		Ask:      &prompt.FakePrompter{},
		FS:       mfs,
		RepoDir:  brewTestRepo,
		Home:     "/home/test",
	}, fr
}

func brewTestMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func brewTestOut(s string) runner.Result { return runner.Result{Stdout: []byte(s)} }

func TestBrewMetadata(t *testing.T) {
	m := NewBrew(func(...string) error { return nil })
	if m.ID() != "brew" {
		t.Errorf("ID = %q, want brew", m.ID())
	}
	if got, want := m.DependsOn(), []module.ID{"repo", "apt"}; !slices.Equal(got, want) {
		t.Errorf("DependsOn = %v, want %v", got, want)
	}
	tests := []struct {
		plat platform.Platform
		want bool
	}{
		{brewTestDarwinARM, true},
		{brewTestDarwinAMD, true},
		{brewTestLinux, true},
		{platform.Platform{OS: platform.Linux, Arch: platform.ARM64}, true},
		{platform.Platform{}, false},
	}
	for _, tt := range tests {
		if got := m.Supports(tt.plat); got != tt.want {
			t.Errorf("Supports(%v) = %v, want %v", tt.plat, got, tt.want)
		}
	}
}

func TestBrewCheck(t *testing.T) {
	tests := []struct {
		name string
		tc   brewTestCase
		want module.Status
		cmds []string
	}{
		{
			name: "darwin without brew runs nothing",
			tc:   brewTestCase{plat: brewTestDarwinARM, casks: true},
			want: module.StatusMissing,
			cmds: nil,
		},
		{
			name: "darwin all installed",
			tc: brewTestCase{plat: brewTestDarwinARM, brewPresent: true, casks: true, script: map[string]runner.Result{
				brewTestDarwinBrew + " list --formula -1": brewTestOut("git\nfish\nrtk\nother\n"),
				brewTestDarwinBrew + " list --cask -1":    brewTestOut("claude-code\nminiforge\n"),
			}},
			want: module.StatusInstalled,
			cmds: []string{brewTestDarwinBrew + " list --formula -1", brewTestDarwinBrew + " list --cask -1"},
		},
		{
			name: "darwin cask missing",
			tc: brewTestCase{plat: brewTestDarwinARM, brewPresent: true, casks: true, script: map[string]runner.Result{
				brewTestDarwinBrew + " list --formula -1": brewTestOut("git\nfish\nrtk\n"),
				brewTestDarwinBrew + " list --cask -1":    brewTestOut("claude-code\n"),
			}},
			want: module.StatusMissing,
			cmds: []string{brewTestDarwinBrew + " list --formula -1", brewTestDarwinBrew + " list --cask -1"},
		},
		{
			name: "linux all installed lists no casks",
			tc: brewTestCase{plat: brewTestLinux, brewPresent: true, script: map[string]runner.Result{
				brewTestLinuxBrew + " list --formula -1": brewTestOut("git\nfish\nrtk\n"),
			}},
			want: module.StatusInstalled,
			cmds: []string{brewTestLinuxBrew + " list --formula -1"},
		},
		{
			name: "linux formula missing",
			tc: brewTestCase{plat: brewTestLinux, brewPresent: true, script: map[string]runner.Result{
				brewTestLinuxBrew + " list --formula -1": brewTestOut("git\n"),
			}},
			want: module.StatusMissing,
			cmds: []string{brewTestLinuxBrew + " list --formula -1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, fr := newBrewTestEnv(t, tt.tc)
			rec := &brewTestPath{}
			got, err := NewBrew(rec.prepend).Check(context.Background(), env)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got != tt.want {
				t.Errorf("Check = %v, want %v", got, tt.want)
			}
			if cmds := fr.Commands(); !slices.Equal(cmds, tt.cmds) {
				t.Errorf("commands = %q, want %q", cmds, tt.cmds)
			}
			if len(rec.calls) != 0 {
				t.Errorf("Check changed PATH: %v", rec.calls)
			}
		})
	}
}

func TestBrewApply(t *testing.T) {
	tests := []struct {
		name        string
		tc          brewTestCase
		cmds        []string
		interactive []bool
		path        []string
	}{
		{
			name: "darwin arm64 installs missing formula and cask",
			tc: brewTestCase{plat: brewTestDarwinARM, brewPresent: true, casks: true, script: map[string]runner.Result{
				brewTestDarwinBrew + " list --formula -1": brewTestOut("git\nrtk\n"),
				brewTestDarwinBrew + " list --cask -1":    brewTestOut("claude-code\n"),
			}},
			cmds: []string{
				brewTestDarwinBrew + " list --formula -1",
				brewTestDarwinBrew + " list --cask -1",
				brewTestDarwinBrew + " install fish",
				brewTestDarwinBrew + " install --cask miniforge",
			},
			interactive: []bool{false, false, false, true},
			path:        []string{"/opt/homebrew/bin", "/opt/homebrew/sbin"},
		},
		{
			name: "darwin amd64 without brew installs Homebrew and everything",
			tc:   brewTestCase{plat: brewTestDarwinAMD, casks: true},
			cmds: []string{
				brewTestInstaller,
				brewTestIntelBrew + " list --formula -1",
				brewTestIntelBrew + " list --cask -1",
				brewTestIntelBrew + " install git fish owner/tap/rtk",
				brewTestIntelBrew + " install --cask claude-code miniforge",
			},
			interactive: []bool{true, false, false, false, true},
			path:        []string{"/usr/local/bin", "/usr/local/sbin"},
		},
		{
			name: "darwin all installed skips installs",
			tc: brewTestCase{plat: brewTestDarwinARM, brewPresent: true, casks: true, script: map[string]runner.Result{
				brewTestDarwinBrew + " list --formula -1": brewTestOut("git\nfish\nrtk\n"),
				brewTestDarwinBrew + " list --cask -1":    brewTestOut("claude-code\nminiforge\n"),
			}},
			cmds: []string{
				brewTestDarwinBrew + " list --formula -1",
				brewTestDarwinBrew + " list --cask -1",
			},
			interactive: []bool{false, false},
			path:        []string{"/opt/homebrew/bin", "/opt/homebrew/sbin"},
		},
		{
			name: "linux without brew installs formulae only",
			tc:   brewTestCase{plat: brewTestLinux},
			cmds: []string{
				brewTestInstaller,
				brewTestLinuxBrew + " list --formula -1",
				brewTestLinuxBrew + " install git fish owner/tap/rtk",
			},
			interactive: []bool{true, false, false},
			path:        []string{"/home/linuxbrew/.linuxbrew/bin", "/home/linuxbrew/.linuxbrew/sbin"},
		},
		{
			name: "linux all installed runs no install",
			tc: brewTestCase{plat: brewTestLinux, brewPresent: true, script: map[string]runner.Result{
				brewTestLinuxBrew + " list --formula -1": brewTestOut("git\nfish\nrtk\n"),
			}},
			cmds:        []string{brewTestLinuxBrew + " list --formula -1"},
			interactive: []bool{false},
			path:        []string{"/home/linuxbrew/.linuxbrew/bin", "/home/linuxbrew/.linuxbrew/sbin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, fr := newBrewTestEnv(t, tt.tc)
			rec := &brewTestPath{}
			if err := NewBrew(rec.prepend).Apply(context.Background(), env); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if cmds := fr.Commands(); !slices.Equal(cmds, tt.cmds) {
				t.Errorf("commands = %q, want %q", cmds, tt.cmds)
			}
			interactive := make([]bool, len(fr.Calls))
			for i, c := range fr.Calls {
				interactive[i] = c.Interactive
			}
			if !slices.Equal(interactive, tt.interactive) {
				t.Errorf("interactive = %v, want %v", interactive, tt.interactive)
			}
			if len(rec.calls) != 1 || !slices.Equal(rec.calls[0], tt.path) {
				t.Errorf("prependPath calls = %v, want one call with %v", rec.calls, tt.path)
			}
		})
	}
}

func TestBrewApplyErrors(t *testing.T) {
	listFail := &runner.CommandError{Cmd: runner.Cmd{Name: brewTestLinuxBrew}, ExitCode: 1}
	installFail := &runner.CommandError{Cmd: runner.Cmd{Name: "/bin/bash"}, ExitCode: 1}
	pathFail := errors.New("setenv failed")

	tests := []struct {
		name    string
		tc      brewTestCase
		pathErr error
		cmds    []string
		check   func(error) bool
	}{
		{
			name:  "installer failure stops before PATH",
			tc:    brewTestCase{plat: brewTestLinux, script: map[string]runner.Result{"/bin/bash": {Err: installFail}}},
			cmds:  []string{brewTestInstaller},
			check: func(err error) bool { var ce *runner.CommandError; return errors.As(err, &ce) && ce == installFail },
		},
		{
			name:    "prependPath failure",
			tc:      brewTestCase{plat: brewTestLinux, brewPresent: true},
			pathErr: pathFail,
			cmds:    nil,
			check:   func(err error) bool { return errors.Is(err, pathFail) },
		},
		{
			name: "brew list failure",
			tc: brewTestCase{plat: brewTestLinux, brewPresent: true, script: map[string]runner.Result{
				brewTestLinuxBrew + " list --formula -1": {Err: listFail},
			}},
			cmds:  []string{brewTestLinuxBrew + " list --formula -1"},
			check: func(err error) bool { var ce *runner.CommandError; return errors.As(err, &ce) && ce == listFail },
		},
		{
			name:  "missing cask list on darwin runs nothing",
			tc:    brewTestCase{plat: brewTestDarwinARM, brewPresent: true},
			cmds:  nil,
			check: func(err error) bool { return errors.Is(err, fs.ErrNotExist) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, fr := newBrewTestEnv(t, tt.tc)
			rec := &brewTestPath{err: tt.pathErr}
			err := NewBrew(rec.prepend).Apply(context.Background(), env)
			if err == nil || !tt.check(err) {
				t.Fatalf("Apply error = %v", err)
			}
			if cmds := fr.Commands(); !slices.Equal(cmds, tt.cmds) {
				t.Errorf("commands = %q, want %q", cmds, tt.cmds)
			}
		})
	}
}

func TestBrewApplyInvalidList(t *testing.T) {
	env, fr := newBrewTestEnv(t, brewTestCase{plat: brewTestLinux, brewPresent: true})
	mfs, ok := env.FS.(*module.MemFS)
	if !ok {
		t.Fatal("env.FS is not *module.MemFS")
	}
	brewTestMust(t, mfs.WriteFile(filepath.Join(brewTestRepo, "packages", "brew.txt"), []byte("bad name\n"), 0o644))
	err := NewBrew(func(...string) error { return nil }).Apply(context.Background(), env)
	var ile *pkglist.InvalidLineError
	if !errors.As(err, &ile) {
		t.Fatalf("Apply error = %v, want *pkglist.InvalidLineError", err)
	}
	if len(fr.Calls) != 0 {
		t.Errorf("commands = %q, want none", fr.Commands())
	}
}
