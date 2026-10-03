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

var (
	claudeDarwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	claudeLinux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
)

const (
	claudeVersionCmd = "claude --version"
	claudeHome       = "/home/u"
)

func newClaudeTestEnv(p platform.Platform, missing bool) (module.Env, *runner.FakeRunner, *module.MemFS) {
	fr := &runner.FakeRunner{Script: map[string]runner.Result{}}
	if missing {
		fr.Script[claudeVersionCmd] = runner.Result{Err: &runner.NotFoundError{Name: "claude"}}
	}
	fsys := &module.MemFS{}
	env := module.Env{Platform: p, Run: fr, Ask: &prompt.FakePrompter{}, FS: fsys, Home: claudeHome}
	return env, fr, fsys
}

func claudeInstallCmd() string {
	return runner.Cmd{Name: "bash", Args: []string{"-c", claudeInstallScript}}.String()
}

func TestClaudeMetadata(t *testing.T) {
	m := NewClaude()
	if m.ID() != "claude" || !slices.Equal(m.DependsOn(), []module.ID{"brew"}) {
		t.Fatalf("id=%q deps=%v", m.ID(), m.DependsOn())
	}
	if !m.Supports(claudeDarwin) || !m.Supports(claudeLinux) || m.Supports(platform.Platform{}) {
		t.Fatal("unexpected Supports result")
	}
}

func TestClaudeCheckInstalled(t *testing.T) {
	for _, p := range []platform.Platform{claudeDarwin, claudeLinux} {
		env, fr, _ := newClaudeTestEnv(p, false)
		st, err := NewClaude().Check(context.Background(), env)
		if err != nil || st != module.StatusInstalled {
			t.Fatalf("%v: st=%v err=%v", p.OS, st, err)
		}
		if got := fr.Commands(); !slices.Equal(got, []string{claudeVersionCmd}) {
			t.Fatalf("commands = %v", got)
		}
	}
}

func TestClaudeCheckLinuxAbsolutePath(t *testing.T) {
	env, _, fsys := newClaudeTestEnv(claudeLinux, true)
	if err := fsys.MkdirAll(claudeHome+"/.local/bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(claudeHome+"/.local/bin/claude", nil, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := NewClaude().Check(context.Background(), env)
	if err != nil || st != module.StatusInstalled {
		t.Fatalf("st=%v err=%v", st, err)
	}
}

func TestClaudeCheckMissing(t *testing.T) {
	for _, p := range []platform.Platform{claudeDarwin, claudeLinux} {
		env, _, _ := newClaudeTestEnv(p, true)
		st, err := NewClaude().Check(context.Background(), env)
		if err != nil || st != module.StatusMissing {
			t.Fatalf("%v: st=%v err=%v", p.OS, st, err)
		}
	}
}

func TestClaudeApplyDarwinMissing(t *testing.T) {
	env, fr, _ := newClaudeTestEnv(claudeDarwin, true)
	err := NewClaude().Apply(context.Background(), env)
	var ce *ClaudeCaskMissingError
	if !errors.As(err, &ce) || ce.Cask != "claude-code@latest" {
		t.Fatalf("err = %v", err)
	}
	if got := fr.Commands(); !slices.Equal(got, []string{claudeVersionCmd}) {
		t.Fatalf("commands = %v", got)
	}
}

func TestClaudeApplyDarwinInstalled(t *testing.T) {
	env, fr, _ := newClaudeTestEnv(claudeDarwin, false)
	if err := NewClaude().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if got := fr.Commands(); !slices.Equal(got, []string{claudeVersionCmd}) {
		t.Fatalf("commands = %v", got)
	}
}

func TestClaudeApplyLinuxInstalls(t *testing.T) {
	env, fr, _ := newClaudeTestEnv(claudeLinux, true)
	if err := NewClaude().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	want := []string{claudeVersionCmd, claudeInstallCmd()}
	if got := fr.Commands(); !slices.Equal(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	if !fr.Calls[1].Interactive {
		t.Fatal("installer must be interactive")
	}
}

func TestClaudeApplyLinuxIdempotent(t *testing.T) {
	env, fr, _ := newClaudeTestEnv(claudeLinux, false)
	if err := NewClaude().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if got := fr.Commands(); !slices.Equal(got, []string{claudeVersionCmd}) {
		t.Fatalf("commands = %v", got)
	}
}

func TestClaudeApplyLinuxInstallFails(t *testing.T) {
	env, fr, _ := newClaudeTestEnv(claudeLinux, true)
	fr.Script[claudeInstallCmd()] = runner.Result{Err: &runner.CommandError{ExitCode: 1}}
	err := NewClaude().Apply(context.Background(), env)
	var cmdErr *runner.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v", err)
	}
}
