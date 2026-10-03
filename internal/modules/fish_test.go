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

const fishTestPath = "/opt/homebrew/bin/fish"

var (
	fishDarwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	fishLinux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
)

var (
	fishWhichCmd = runner.Cmd{Name: "sh", Args: []string{"-c", "command -v fish"}}
	fishDsclCmd  = runner.Cmd{Name: "dscl", Args: []string{".", "-read", "/Users/tyson", "UserShell"}}
)

type fishTestEnv struct {
	env module.Env
	run *runner.FakeRunner
	ask *prompt.FakePrompter
}

// newFishTestEnv scripts a machine where fish is at fishTestPath, the user is
// "tyson", the login shell is shell, and /etc/shells holds shells.
func newFishTestEnv(t *testing.T, p platform.Platform, shells, shell string, fisher bool, confirms ...bool) fishTestEnv {
	t.Helper()
	script := map[string]runner.Result{
		fishWhichCmd.String(): {Stdout: []byte(fishTestPath + "\n")},
		"id -un":              {Stdout: []byte("tyson\n")},
		fishDsclCmd.String():  {Stdout: []byte("UserShell: " + shell + "\n")},
		"getent passwd tyson": {Stdout: []byte("tyson:x:1000:1000:Tyson:/home/tyson:" + shell + "\n")},
	}
	if !fisher {
		script["fish -c 'type -q fisher'"] = runner.Result{Err: &runner.CommandError{ExitCode: 1}}
	}
	fsys := &module.MemFS{}
	if err := fsys.MkdirAll("/etc", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile("/etc/shells", []byte(shells), 0o644); err != nil {
		t.Fatal(err)
	}
	run := &runner.FakeRunner{Script: script}
	ask := &prompt.FakePrompter{Confirms: confirms}
	return fishTestEnv{
		env: module.Env{Platform: p, Run: run, Ask: ask, FS: fsys, Home: "/home/tyson", RepoDir: "/home/tyson/dotfiles"},
		run: run, ask: ask,
	}
}

// fishShellCmds are the commands that read the login shell.
func fishShellCmds(p platform.Platform) []string {
	if p.OS == platform.Darwin {
		return []string{"id -un", "dscl . -read /Users/tyson UserShell"}
	}
	return []string{"id -un", "getent passwd tyson"}
}

func TestFishMetadata(t *testing.T) {
	m := NewFish()
	if m.ID() != "fish" || !slices.Equal(m.DependsOn(), []module.ID{"brew", "chezmoi"}) {
		t.Fatalf("id=%q deps=%v", m.ID(), m.DependsOn())
	}
	if !m.Supports(fishDarwin) || !m.Supports(fishLinux) {
		t.Fatal("must support darwin and linux")
	}
}

func TestFishAlreadyDefault(t *testing.T) {
	for name, p := range map[string]platform.Platform{"darwin": fishDarwin, "linux": fishLinux} {
		t.Run(name, func(t *testing.T) {
			e := newFishTestEnv(t, p, "/bin/sh\n"+fishTestPath+"\n", fishTestPath, true)
			st, err := NewFish().Check(context.Background(), e.env)
			if err != nil || st != module.StatusInstalled {
				t.Fatalf("check = %v, %v", st, err)
			}
			want := append([]string{"sh -c 'command -v fish'"}, fishShellCmds(p)...)
			want = append(want, "fish -c 'type -q fisher'")
			if got := e.run.Commands(); !slices.Equal(got, want) {
				t.Fatalf("check commands = %q, want %q", got, want)
			}

			e.run.Calls = nil
			if err := NewFish().Apply(context.Background(), e.env); err != nil {
				t.Fatal(err)
			}
			want = append([]string{"sh -c 'command -v fish'"}, fishShellCmds(p)...)
			want = append(want, "fish -c 'fisher update'")
			if got := e.run.Commands(); !slices.Equal(got, want) {
				t.Fatalf("apply commands = %q, want %q", got, want)
			}
			if len(e.ask.Asked) != 0 {
				t.Fatalf("asked %q, want no prompt", e.ask.Asked)
			}
		})
	}
}

func TestFishAppendsToShells(t *testing.T) {
	for name, p := range map[string]platform.Platform{"darwin": fishDarwin, "linux": fishLinux} {
		t.Run(name, func(t *testing.T) {
			e := newFishTestEnv(t, p, "/bin/sh\n", "/bin/zsh", true, true)
			if st, err := NewFish().Check(context.Background(), e.env); err != nil || st != module.StatusMissing {
				t.Fatalf("check = %v, %v", st, err)
			}
			e.run.Calls = nil
			if err := NewFish().Apply(context.Background(), e.env); err != nil {
				t.Fatal(err)
			}
			sudo := runner.Cmd{Name: "sudo", Args: []string{"sh", "-c", "echo '" + fishTestPath + "' >> /etc/shells"}}.String()
			want := []string{"sh -c 'command -v fish'", sudo}
			want = append(want, fishShellCmds(p)...)
			chsh := "chsh -s " + fishTestPath
			if p.OS == platform.Linux {
				chsh = "sudo chsh -s " + fishTestPath + " tyson"
			}
			want = append(want, chsh, "fish -c 'fisher update'")
			if got := e.run.Commands(); !slices.Equal(got, want) {
				t.Fatalf("commands = %q, want %q", got, want)
			}
			for _, c := range e.run.Calls {
				if (c.Name == "sudo" || c.Name == "chsh") != c.Interactive {
					t.Fatalf("%s Interactive = %v", c.Name, c.Interactive)
				}
			}
			if !slices.Equal(e.ask.Asked, []string{"Make fish your login shell?"}) {
				t.Fatalf("asked = %q", e.ask.Asked)
			}
		})
	}
}

func TestFishDeclineChsh(t *testing.T) {
	e := newFishTestEnv(t, fishLinux, fishTestPath+"\n", "/bin/bash", true, false)
	if err := NewFish().Apply(context.Background(), e.env); err != nil {
		t.Fatal(err)
	}
	want := append([]string{"sh -c 'command -v fish'"}, fishShellCmds(fishLinux)...)
	want = append(want, "fish -c 'fisher update'")
	if got := e.run.Commands(); !slices.Equal(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestFishCheckMissingFisher(t *testing.T) {
	e := newFishTestEnv(t, fishDarwin, fishTestPath+"\n", fishTestPath, false)
	if st, err := NewFish().Check(context.Background(), e.env); err != nil || st != module.StatusMissing {
		t.Fatalf("check = %v, %v", st, err)
	}
}

func TestFishCheckNoFish(t *testing.T) {
	e := newFishTestEnv(t, fishDarwin, "", "", true)
	e.run.Script[fishWhichCmd.String()] = runner.Result{Err: &runner.CommandError{ExitCode: 1}}
	if st, err := NewFish().Check(context.Background(), e.env); err != nil || st != module.StatusMissing {
		t.Fatalf("check = %v, %v", st, err)
	}
}

func TestFishBadLoginShellOutput(t *testing.T) {
	e := newFishTestEnv(t, fishDarwin, fishTestPath+"\n", "", true)
	e.run.Script[fishDsclCmd.String()] = runner.Result{Stdout: []byte("garbage\n")}
	_, err := NewFish().Check(context.Background(), e.env)
	var pe *fishParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *fishParseError", err)
	}
}
