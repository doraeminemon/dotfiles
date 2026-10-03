package modules

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

var (
	aptTestLinux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
	aptTestDarwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
)

func aptTestQuery(pkg string) string {
	return runner.Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", pkg}}.String()
}

func newAptTestEnv(t *testing.T, p platform.Platform, list string, script map[string]runner.Result) (module.Env, *runner.FakeRunner) {
	t.Helper()
	fake := &runner.FakeRunner{Script: script}
	fsys := &module.MemFS{}
	if list != "" {
		if err := fsys.MkdirAll("/repo/packages", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fsys.WriteFile("/repo/packages/apt.txt", []byte(list), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return module.Env{
		Platform: p, Run: fake, Ask: &prompt.FakePrompter{}, FS: fsys,
		RepoDir: "/repo", Home: "/home/u",
	}, fake
}

func TestAptSupports(t *testing.T) {
	m := NewApt()
	tests := []struct {
		name string
		p    platform.Platform
		want bool
	}{
		{"linux apt", aptTestLinux, true},
		{"darwin", aptTestDarwin, false},
		{"linux without apt", platform.Platform{OS: platform.Linux, Arch: platform.AMD64}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Supports(tt.p); got != tt.want {
				t.Fatalf("Supports = %v, want %v", got, tt.want)
			}
		})
	}
	if m.ID() != "apt" || len(m.DependsOn()) != 0 {
		t.Fatalf("ID/DependsOn = %q/%v", m.ID(), m.DependsOn())
	}
}

func TestAptCheckAndApply(t *testing.T) {
	const list = "# c\ncurl\ngit\nfish\n"
	installed := runner.Result{Stdout: []byte("install ok installed")}
	notFound := runner.Result{Err: &runner.CommandError{}}
	removed := runner.Result{Stdout: []byte("deinstall ok config-files")}
	checks := []string{aptTestQuery("curl"), aptTestQuery("git"), aptTestQuery("fish")}
	tests := []struct {
		name       string
		script     map[string]runner.Result
		wantStatus module.Status
		wantApply  []string
	}{
		{
			name: "all installed",
			script: map[string]runner.Result{
				aptTestQuery("curl"): installed, aptTestQuery("git"): installed, aptTestQuery("fish"): installed,
			},
			wantStatus: module.StatusInstalled,
		},
		{
			name: "some missing",
			script: map[string]runner.Result{
				aptTestQuery("curl"): installed, aptTestQuery("git"): notFound, aptTestQuery("fish"): removed,
			},
			wantStatus: module.StatusMissing,
			wantApply: []string{
				"sudo apt-get update",
				"sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y git fish",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, fake := newAptTestEnv(t, aptTestLinux, list, tt.script)
			got, err := NewApt().Check(context.Background(), env)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.wantStatus {
				t.Fatalf("status = %v, want %v", got, tt.wantStatus)
			}
			if !slices.Equal(fake.Commands(), checks) {
				t.Fatalf("check commands = %v, want %v", fake.Commands(), checks)
			}
			for _, c := range fake.Calls {
				if c.Interactive {
					t.Fatalf("check ran interactive command %v", c)
				}
			}

			env, fake = newAptTestEnv(t, aptTestLinux, list, tt.script)
			if err := NewApt().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			want := append(slices.Clone(checks), tt.wantApply...)
			if !slices.Equal(fake.Commands(), want) {
				t.Fatalf("apply commands = %v, want %v", fake.Commands(), want)
			}
			for _, c := range fake.Calls[len(checks):] {
				if !c.Interactive {
					t.Fatalf("sudo command must be interactive: %v", c)
				}
			}
		})
	}
}

func TestAptApplyBuiltInListInstallsAll(t *testing.T) {
	env, fake := newAptTestEnv(t, aptTestLinux, "", map[string]runner.Result{"dpkg-query": {Err: &runner.CommandError{}}})
	if err := NewApt().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	cmds := fake.Commands()
	want := "sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y build-essential curl git file procps unzip ca-certificates fish"
	if got := cmds[len(cmds)-1]; got != want {
		t.Fatalf("install = %q, want %q", got, want)
	}
}

func TestAptApplyUpdateFailure(t *testing.T) {
	boom := errors.New("boom")
	env, fake := newAptTestEnv(t, aptTestLinux, "curl\n", map[string]runner.Result{
		aptTestQuery("curl"):  {Err: &runner.CommandError{}},
		"sudo apt-get update": {Err: boom},
	})
	err := NewApt().Apply(context.Background(), env)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping boom", err)
	}
	if len(fake.Commands()) != 2 {
		t.Fatalf("commands = %v", fake.Commands())
	}
}

func TestAptDefaultListMatchesRepoFile(t *testing.T) {
	data, err := os.ReadFile("../../packages/apt.txt")
	if err != nil {
		t.Fatal(err)
	}
	env, _ := newAptTestEnv(t, aptTestLinux, string(data), nil)
	fromFile, err := aptPackages(env)
	if err != nil {
		t.Fatal(err)
	}
	env, _ = newAptTestEnv(t, aptTestLinux, "", nil)
	fromConst, err := aptPackages(env)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromFile, fromConst) {
		t.Fatalf("aptDefaultList %v != packages/apt.txt %v", fromConst, fromFile)
	}
}
