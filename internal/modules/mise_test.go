package modules

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	miseTestHome   = "/home/test"
	miseListCmd    = "mise ls --missing --global"
	miseInstallCmd = "mise install --yes"
)

var miseTestPlatforms = map[string]platform.Platform{
	"darwin": {OS: platform.Darwin, Arch: platform.ARM64},
	"linux":  {OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt},
}

func newMiseTestEnv(p platform.Platform, fr *runner.FakeRunner) module.Env {
	return module.Env{
		Platform: p,
		Run:      fr,
		Ask:      &prompt.FakePrompter{},
		FS:       &module.MemFS{},
		RepoDir:  miseTestHome + "/repo",
		Home:     miseTestHome,
	}
}

func TestMiseMetadata(t *testing.T) {
	m := NewMise()
	if m.ID() != "mise" {
		t.Errorf("ID = %q", m.ID())
	}
	if want := []module.ID{"brew", "chezmoi"}; !slices.Equal(m.DependsOn(), want) {
		t.Errorf("DependsOn = %v, want %v", m.DependsOn(), want)
	}
	for name, p := range miseTestPlatforms {
		if !m.Supports(p) {
			t.Errorf("Supports(%s) = false", name)
		}
	}
}

func TestMiseCheck(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		res     runner.Result
		want    module.Status
		wantErr bool
	}{
		{"empty output is installed", runner.Result{}, module.StatusInstalled, false},
		{"whitespace is installed", runner.Result{Stdout: []byte("\n")}, module.StatusInstalled, false},
		{"missing tools", runner.Result{Stdout: []byte("node  22.0.0  ~/.config/mise/config.toml  22  (missing)\n")}, module.StatusMissing, false},
		{"command error", runner.Result{Err: boom}, module.StatusMissing, true},
	}
	for osName, p := range miseTestPlatforms {
		for _, tc := range tests {
			t.Run(osName+"/"+tc.name, func(t *testing.T) {
				fr := &runner.FakeRunner{Script: map[string]runner.Result{miseListCmd: tc.res}}
				got, err := NewMise().Check(context.Background(), newMiseTestEnv(p, fr))
				if (err != nil) != tc.wantErr {
					t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
				}
				if tc.wantErr && !errors.Is(err, boom) {
					t.Errorf("err %v does not wrap boom", err)
				}
				if got != tc.want {
					t.Errorf("status = %v, want %v", got, tc.want)
				}
				if cmds := fr.Commands(); !slices.Equal(cmds, []string{miseListCmd}) {
					t.Errorf("commands = %v", cmds)
				}
				if fr.Calls[0].Dir != miseTestHome {
					t.Errorf("Dir = %q", fr.Calls[0].Dir)
				}
			})
		}
	}
}

func TestMiseApply(t *testing.T) {
	for osName, p := range miseTestPlatforms {
		t.Run(osName, func(t *testing.T) {
			fr := &runner.FakeRunner{}
			if err := NewMise().Apply(context.Background(), newMiseTestEnv(p, fr)); err != nil {
				t.Fatal(err)
			}
			if cmds := fr.Commands(); !slices.Equal(cmds, []string{miseInstallCmd}) {
				t.Errorf("commands = %v", cmds)
			}
			if fr.Calls[0].Dir != miseTestHome || fr.Calls[0].Interactive {
				t.Errorf("call = %+v", fr.Calls[0])
			}
		})
	}
}

func TestMiseApplyError(t *testing.T) {
	boom := errors.New("boom")
	fr := &runner.FakeRunner{Script: map[string]runner.Result{miseInstallCmd: {Err: boom}}}
	err := NewMise().Apply(context.Background(), newMiseTestEnv(miseTestPlatforms["darwin"], fr))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping boom", err)
	}
}
