package modules

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	chezmoiTestHome = "/home/ada"
	chezmoiTestRepo = "/home/ada/Projects/dotfiles"
)

var chezmoiTestPlatforms = map[string]platform.Platform{
	"darwin": {OS: platform.Darwin, Arch: platform.ARM64},
	"linux":  {OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt},
}

var chezmoiTestNow = time.Date(2026, 10, 3, 14, 5, 9, 0, time.UTC)

type chezmoiTestEnv struct {
	env    module.Env
	run    *runner.FakeRunner
	ask    *prompt.FakePrompter
	fs     *module.MemFS
	module module.Module
}

func newChezmoiTestEnv(t *testing.T, p platform.Platform, withConfig bool) chezmoiTestEnv {
	t.Helper()
	mem := &module.MemFS{}
	if err := mem.MkdirAll(chezmoiTestRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if withConfig {
		chezmoiTestWrite(t, mem, filepath.Join(chezmoiTestHome, ".config/chezmoi/chezmoi.toml"), "x", 0o600)
	}
	run := &runner.FakeRunner{Script: map[string]runner.Result{}}
	ask := &prompt.FakePrompter{}
	return chezmoiTestEnv{
		env: module.Env{
			Platform: p,
			Run:      run,
			Ask:      ask,
			FS:       mem,
			RepoDir:  chezmoiTestRepo,
			Home:     chezmoiTestHome,
		},
		run:    run,
		ask:    ask,
		fs:     mem,
		module: NewChezmoi(func() time.Time { return chezmoiTestNow }),
	}
}

func chezmoiTestWrite(t *testing.T, mem *module.MemFS, path, data string, perm fs.FileMode) {
	t.Helper()
	if err := mem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile(path, []byte(data), perm); err != nil {
		t.Fatal(err)
	}
}

const (
	chezmoiTestStatus  = "chezmoi --source /home/ada/Projects/dotfiles status"
	chezmoiTestDiff    = "chezmoi --source /home/ada/Projects/dotfiles diff"
	chezmoiTestManaged = "chezmoi --source /home/ada/Projects/dotfiles managed --path-style absolute"
	chezmoiTestApply   = "chezmoi --source /home/ada/Projects/dotfiles apply"
	chezmoiTestInit    = "chezmoi --source /home/ada/Projects/dotfiles init --promptString name=Ada --promptString email=ada@example.com"
)

func chezmoiAssertCommands(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("commands:\n got  %q\n want %q", got, want)
	}
}

func TestChezmoiMetadata(t *testing.T) {
	m := NewChezmoi(time.Now)
	if m.ID() != "chezmoi" {
		t.Fatalf("ID = %q", m.ID())
	}
	if !slices.Equal(m.DependsOn(), []module.ID{"repo", "brew"}) {
		t.Fatalf("DependsOn = %v", m.DependsOn())
	}
	for name, p := range chezmoiTestPlatforms {
		if !m.Supports(p) {
			t.Errorf("%s: Supports = false", name)
		}
	}
}

func TestChezmoiCheck(t *testing.T) {
	for name, p := range chezmoiTestPlatforms {
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				name       string
				withConfig bool
				status     runner.Result
				want       module.Status
				wantCmds   []string
			}{
				{"clean with config", true, runner.Result{}, module.StatusInstalled, []string{chezmoiTestStatus}},
				{"pending changes", true, runner.Result{Stdout: []byte(" M .config/fish/config.fish\n")}, module.StatusMissing, []string{chezmoiTestStatus}},
				{"no config", false, runner.Result{}, module.StatusMissing, []string{}},
				// Without a config the templates fail on missing data, so
				// status must not run at all.
				{"no config, status would fail", false, runner.Result{Err: &runner.CommandError{ExitCode: 1}}, module.StatusMissing, []string{}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					te := newChezmoiTestEnv(t, p, tc.withConfig)
					te.run.Script[chezmoiTestStatus] = tc.status
					got, err := te.module.Check(context.Background(), te.env)
					if err != nil {
						t.Fatal(err)
					}
					if got != tc.want {
						t.Fatalf("status = %v, want %v", got, tc.want)
					}
					chezmoiAssertCommands(t, te.run.Commands(), tc.wantCmds)
				})
			}
		})
	}
}

func TestChezmoiCheckStatusError(t *testing.T) {
	te := newChezmoiTestEnv(t, chezmoiTestPlatforms["linux"], true)
	cmdErr := &runner.CommandError{ExitCode: 1}
	te.run.Script[chezmoiTestStatus] = runner.Result{Err: cmdErr}
	_, err := te.module.Check(context.Background(), te.env)
	var target *runner.CommandError
	if !errors.As(err, &target) {
		t.Fatalf("err = %v, want *runner.CommandError", err)
	}
}

func TestChezmoiApplyFreshInitAndBackup(t *testing.T) {
	for name, p := range chezmoiTestPlatforms {
		t.Run(name, func(t *testing.T) {
			te := newChezmoiTestEnv(t, p, false)
			fishConfig := filepath.Join(chezmoiTestHome, ".config/fish/config.fish")
			gitconfig := filepath.Join(chezmoiTestHome, ".gitconfig")
			sshConfig := filepath.Join(chezmoiTestHome, ".ssh/config")
			chezmoiTestWrite(t, te.fs, fishConfig, "old fish", 0o644)
			chezmoiTestWrite(t, te.fs, gitconfig, "old git", 0o644)
			chezmoiTestWrite(t, te.fs, sshConfig, "old ssh", 0o600)
			managed := fishConfig + "\n" +
				filepath.Join(chezmoiTestHome, ".config/fish") + "\n" + // directory: skipped
				filepath.Join(chezmoiTestHome, ".config/starship.toml") + "\n" + // missing: skipped
				gitconfig + "\n" + sshConfig + "\n"
			te.run.Script[chezmoiTestDiff] = runner.Result{Stdout: []byte("diff --git a/x b/x\n")}
			te.run.Script[chezmoiTestManaged] = runner.Result{Stdout: []byte(managed)}
			te.ask.Inputs = []string{"Ada", "ada@example.com"}
			te.ask.Confirms = []bool{true}

			if err := te.module.Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			chezmoiAssertCommands(t, te.run.Commands(), []string{
				chezmoiTestInit, chezmoiTestDiff, chezmoiTestManaged, chezmoiTestApply,
			})
			if !te.run.Calls[3].Interactive {
				t.Error("apply is not Interactive")
			}
			if te.run.Calls[0].Interactive {
				t.Error("init is Interactive")
			}

			backup := filepath.Join(chezmoiTestHome, ".dotfiles-backup", "20261003-140509")
			for rel, want := range map[string]struct {
				data string
				perm fs.FileMode
			}{
				".config/fish/config.fish": {"old fish", 0o644},
				".gitconfig":               {"old git", 0o644},
				".ssh/config":              {"old ssh", 0o600},
			} {
				path := filepath.Join(backup, rel)
				data, err := te.fs.ReadFile(path)
				if err != nil {
					t.Fatalf("backup %s: %v", rel, err)
				}
				if string(data) != want.data {
					t.Errorf("backup %s = %q, want %q", rel, data, want.data)
				}
				info, err := te.fs.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != want.perm {
					t.Errorf("backup %s mode = %v, want %v", rel, info.Mode().Perm(), want.perm)
				}
			}
			if _, err := te.fs.Stat(filepath.Join(backup, ".config/starship.toml")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("backup of missing target: err = %v, want not exist", err)
			}
		})
	}
}

func TestChezmoiApplyDeclineAtDiff(t *testing.T) {
	for name, p := range chezmoiTestPlatforms {
		t.Run(name, func(t *testing.T) {
			te := newChezmoiTestEnv(t, p, true)
			te.run.Script[chezmoiTestDiff] = runner.Result{Stdout: []byte("diff --git a/x b/x\n")}
			te.ask.Confirms = []bool{false}

			if err := te.module.Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			chezmoiAssertCommands(t, te.run.Commands(), []string{chezmoiTestDiff})
			if !slices.Equal(te.ask.Asked, []string{"Apply these changes to $HOME?"}) {
				t.Fatalf("asked = %q", te.ask.Asked)
			}
			if _, err := te.fs.Stat(filepath.Join(chezmoiTestHome, ".dotfiles-backup")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("backup dir: err = %v, want not exist", err)
			}
		})
	}
}

func TestChezmoiApplyExistingConfigEmptyDiff(t *testing.T) {
	for name, p := range chezmoiTestPlatforms {
		t.Run(name, func(t *testing.T) {
			te := newChezmoiTestEnv(t, p, true)

			if err := te.module.Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			chezmoiAssertCommands(t, te.run.Commands(), []string{
				chezmoiTestDiff, chezmoiTestManaged, chezmoiTestApply,
			})
			if len(te.ask.Asked) != 0 {
				t.Fatalf("asked = %q, want no questions", te.ask.Asked)
			}
		})
	}
}

func TestChezmoiApplyYesSkipsConfirm(t *testing.T) {
	te := newChezmoiTestEnv(t, chezmoiTestPlatforms["darwin"], true)
	te.env.Yes = true
	te.run.Script[chezmoiTestDiff] = runner.Result{Stdout: []byte("diff\n")}

	if err := te.module.Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	chezmoiAssertCommands(t, te.run.Commands(), []string{
		chezmoiTestDiff, chezmoiTestManaged, chezmoiTestApply,
	})
}

func TestChezmoiApplyInvalidEmail(t *testing.T) {
	te := newChezmoiTestEnv(t, chezmoiTestPlatforms["linux"], false)
	te.ask.Inputs = []string{"Ada", "not-an-email"}

	err := te.module.Apply(context.Background(), te.env)
	if !errors.Is(err, errChezmoiInvalidEmail) {
		t.Fatalf("err = %v, want errChezmoiInvalidEmail", err)
	}
	chezmoiAssertCommands(t, te.run.Commands(), []string{})
}

func TestChezmoiApplyManagedOutsideHome(t *testing.T) {
	te := newChezmoiTestEnv(t, chezmoiTestPlatforms["linux"], true)
	chezmoiTestWrite(t, te.fs, "/etc/outside", "x", 0o644)
	te.run.Script[chezmoiTestManaged] = runner.Result{Stdout: []byte("/etc/outside\n")}

	err := te.module.Apply(context.Background(), te.env)
	var target *ChezmoiOutsideHomeError
	if !errors.As(err, &target) {
		t.Fatalf("err = %v, want *ChezmoiOutsideHomeError", err)
	}
	chezmoiAssertCommands(t, te.run.Commands(), []string{chezmoiTestDiff, chezmoiTestManaged})
}

func TestChezmoiApplySkipsSymlinkTargets(t *testing.T) {
	te := newChezmoiTestEnv(t, chezmoiTestPlatforms["darwin"], true)
	repoFile := filepath.Join(chezmoiTestRepo, "home/dot_config/fish/config.fish")
	chezmoiTestWrite(t, te.fs, repoFile, "repo fish", 0o644)
	fishConfig := filepath.Join(chezmoiTestHome, ".config/fish/config.fish")
	if err := te.fs.MkdirAll(filepath.Dir(fishConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := te.fs.Symlink(repoFile, fishConfig); err != nil {
		t.Fatal(err)
	}
	te.run.Script[chezmoiTestDiff] = runner.Result{Stdout: []byte("diff\n")}
	te.run.Script[chezmoiTestManaged] = runner.Result{Stdout: []byte(fishConfig + "\n")}
	te.env.Yes = true

	if err := te.module.Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(chezmoiTestHome, ".dotfiles-backup")
	if _, err := te.fs.Stat(backupRoot); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup root exists for a symlink-only run (err=%v)", err)
	}
}
