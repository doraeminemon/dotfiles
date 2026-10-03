package modules

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const pythonTestHome = "/home/tester"

var (
	pythonDarwinARM  = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	pythonDarwinAMD  = platform.Platform{OS: platform.Darwin, Arch: platform.AMD64}
	pythonLinuxAMD   = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
	pythonLinuxARM   = platform.Platform{OS: platform.Linux, Arch: platform.ARM64, PkgMgr: platform.Apt}
	pythonBothTools  = []byte("ruff v0.6.9\n- ruff\nty v0.0.1\n- ty\n")
	pythonOnlyRuff   = []byte("ruff v0.6.9\n- ruff\n")
	pythonLinuxConda = pythonTestHome + "/miniforge3/bin/conda"
)

func newPythonTestEnv(t *testing.T, p platform.Platform, r *runner.FakeRunner) (module.Env, *module.MemFS) {
	t.Helper()
	fsys := &module.MemFS{}
	if err := fsys.MkdirAll(pythonTestHome, 0o755); err != nil {
		t.Fatal(err)
	}
	return module.Env{
		Platform: p,
		Run:      r,
		Ask:      &prompt.FakePrompter{},
		FS:       fsys,
		RepoDir:  pythonTestHome + "/Projects/dotfiles",
		Home:     pythonTestHome,
	}, fsys
}

func pythonPutConda(t *testing.T, fsys *module.MemFS, path string) {
	t.Helper()
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func pythonAssertCommands(t *testing.T, r *runner.FakeRunner, want []string) {
	t.Helper()
	if got := r.Commands(); !slices.Equal(got, want) {
		t.Fatalf("commands:\n got  %q\n want %q", got, want)
	}
}

func TestPythonMetadata(t *testing.T) {
	m := NewPython()
	if m.ID() != "python" {
		t.Errorf("ID = %q", m.ID())
	}
	if !slices.Equal(m.DependsOn(), []module.ID{"brew"}) {
		t.Errorf("DependsOn = %v", m.DependsOn())
	}
	for _, p := range []platform.Platform{pythonDarwinARM, pythonDarwinAMD, pythonLinuxAMD, pythonLinuxARM} {
		if !m.Supports(p) {
			t.Errorf("Supports(%v) = false", p)
		}
	}
	if m.Supports(platform.Platform{}) {
		t.Error("Supports(zero Platform) = true")
	}
}

func TestPythonCheck(t *testing.T) {
	tests := []struct {
		name      string
		p         platform.Platform
		conda     string
		uvList    []byte
		want      module.Status
		wantCalls []string
	}{
		{
			name:      "darwin arm64 installed",
			p:         pythonDarwinARM,
			conda:     "/opt/homebrew/Caskroom/miniforge/base/bin/conda",
			uvList:    pythonBothTools,
			want:      module.StatusInstalled,
			wantCalls: []string{"uv tool list"},
		},
		{
			name:      "darwin amd64 installed",
			p:         pythonDarwinAMD,
			conda:     "/usr/local/Caskroom/miniforge/base/bin/conda",
			uvList:    pythonBothTools,
			want:      module.StatusInstalled,
			wantCalls: []string{"uv tool list"},
		},
		{
			name:      "darwin conda missing",
			p:         pythonDarwinARM,
			want:      module.StatusMissing,
			wantCalls: nil,
		},
		{
			name:      "linux installed",
			p:         pythonLinuxAMD,
			conda:     pythonLinuxConda,
			uvList:    pythonBothTools,
			want:      module.StatusInstalled,
			wantCalls: []string{"uv tool list"},
		},
		{
			name:      "linux ty missing",
			p:         pythonLinuxARM,
			conda:     pythonLinuxConda,
			uvList:    pythonOnlyRuff,
			want:      module.StatusMissing,
			wantCalls: []string{"uv tool list"},
		},
		{
			name: "linux conda in the darwin place only",
			p:    pythonLinuxAMD,
			// The linux prefix is under Home, not the Caskroom.
			conda: "/opt/homebrew/Caskroom/miniforge/base/bin/conda",
			want:  module.StatusMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner.FakeRunner{Script: map[string]runner.Result{
				"uv tool list": {Stdout: tt.uvList},
			}}
			env, fsys := newPythonTestEnv(t, tt.p, r)
			if tt.conda != "" {
				pythonPutConda(t, fsys, tt.conda)
			}
			got, err := NewPython().Check(context.Background(), env)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Check = %v, want %v", got, tt.want)
			}
			pythonAssertCommands(t, r, tt.wantCalls)
		})
	}
}

func TestPythonCheckUVNotOnPath(t *testing.T) {
	r := &runner.FakeRunner{Script: map[string]runner.Result{
		"uv tool list": {Err: &runner.NotFoundError{Name: "uv"}},
	}}
	env, fsys := newPythonTestEnv(t, pythonDarwinARM, r)
	pythonPutConda(t, fsys, "/opt/homebrew/Caskroom/miniforge/base/bin/conda")
	got, err := NewPython().Check(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got != module.StatusMissing {
		t.Errorf("Check = %v, want missing", got)
	}
}

func TestPythonCheckUVFails(t *testing.T) {
	boom := &runner.CommandError{Cmd: runner.Cmd{Name: "uv"}, ExitCode: 2}
	r := &runner.FakeRunner{Script: map[string]runner.Result{"uv tool list": {Err: boom}}}
	env, fsys := newPythonTestEnv(t, pythonLinuxAMD, r)
	pythonPutConda(t, fsys, pythonLinuxConda)
	_, err := NewPython().Check(context.Background(), env)
	var ce *runner.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("Check err = %v, want *runner.CommandError", err)
	}
}

func TestPythonApplyDarwin(t *testing.T) {
	tests := []struct {
		name string
		p    platform.Platform
		base string
	}{
		{"arm64", pythonDarwinARM, "/opt/homebrew/Caskroom/miniforge/base"},
		{"amd64", pythonDarwinAMD, "/usr/local/Caskroom/miniforge/base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner.FakeRunner{}
			env, _ := newPythonTestEnv(t, tt.p, r)
			if err := NewPython().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			pythonAssertCommands(t, r, []string{
				tt.base + "/bin/conda init fish",
				tt.base + "/bin/conda config --set auto_activate_base false",
				"uv tool list",
				"uv tool install ruff",
				"uv tool install ty",
			})
		})
	}
}

func TestPythonApplyLinuxInstallsMiniforge(t *testing.T) {
	tests := []struct {
		name    string
		p       platform.Platform
		machine string
	}{
		{"amd64", pythonLinuxAMD, "x86_64"},
		{"arm64", pythonLinuxARM, "aarch64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner.FakeRunner{}
			env, fsys := newPythonTestEnv(t, tt.p, r)
			if err := NewPython().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			url := "https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-" + tt.machine + ".sh"
			tmp := "/home/tester/.cache/miniforge-installer.sh"
			pythonAssertCommands(t, r, []string{
				"sh -c 'curl -fsSL " + url + " -o " + tmp +
					" && bash " + tmp + " -b -p /home/tester/miniforge3 && rm -f " + tmp + "'",
				pythonLinuxConda + " init fish",
				pythonLinuxConda + " config --set auto_activate_base false",
				"uv tool list",
				"uv tool install ruff",
				"uv tool install ty",
			})
			if !r.Calls[0].Interactive {
				t.Error("installer is not Interactive")
			}
			info, err := fsys.Stat("/home/tester/.cache")
			if err != nil || !info.IsDir() {
				t.Errorf("installer dir not created: %v", err)
			}
		})
	}
}

func TestPythonApplyLinuxAlreadyInstalled(t *testing.T) {
	r := &runner.FakeRunner{Script: map[string]runner.Result{
		"uv tool list": {Stdout: pythonBothTools},
	}}
	env, fsys := newPythonTestEnv(t, pythonLinuxARM, r)
	pythonPutConda(t, fsys, pythonLinuxConda)
	if err := NewPython().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	pythonAssertCommands(t, r, []string{
		pythonLinuxConda + " init fish",
		pythonLinuxConda + " config --set auto_activate_base false",
		"uv tool list",
	})
}

func TestPythonApplyInstallsOnlyMissingTool(t *testing.T) {
	r := &runner.FakeRunner{Script: map[string]runner.Result{
		"uv tool list": {Stdout: pythonOnlyRuff},
	}}
	env, _ := newPythonTestEnv(t, pythonDarwinARM, r)
	if err := NewPython().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	conda := "/opt/homebrew/Caskroom/miniforge/base/bin/conda"
	pythonAssertCommands(t, r, []string{
		conda + " init fish",
		conda + " config --set auto_activate_base false",
		"uv tool list",
		"uv tool install ty",
	})
}

func TestPythonApplyInstallerFails(t *testing.T) {
	boom := &runner.CommandError{Cmd: runner.Cmd{Name: "sh"}, ExitCode: 22}
	r := &runner.FakeRunner{Script: map[string]runner.Result{"sh": {Err: boom}}}
	env, _ := newPythonTestEnv(t, pythonLinuxAMD, r)
	err := NewPython().Apply(context.Background(), env)
	var ce *runner.CommandError
	if !errors.As(err, &ce) || ce.ExitCode != 22 {
		t.Fatalf("Apply err = %v, want the installer *runner.CommandError", err)
	}
	if n := len(r.Calls); n != 1 {
		t.Errorf("ran %d commands after the failed installer, want 1", n)
	}
}

func TestPythonApplyQuotesHomeWithSpace(t *testing.T) {
	r := &runner.FakeRunner{}
	env, fsys := newPythonTestEnv(t, pythonLinuxAMD, r)
	env.Home = "/home/my user"
	if err := fsys.MkdirAll(env.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewPython().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	want := "curl -fsSL https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-x86_64.sh" +
		" -o '/home/my user/.cache/miniforge-installer.sh'" +
		" && bash '/home/my user/.cache/miniforge-installer.sh' -b -p '/home/my user/miniforge3'" +
		" && rm -f '/home/my user/.cache/miniforge-installer.sh'"
	if got := r.Calls[0].Args[1]; got != want {
		t.Errorf("script:\n got  %s\n want %s", got, want)
	}
}

func TestPythonApplyUnsupported(t *testing.T) {
	r := &runner.FakeRunner{}
	env, _ := newPythonTestEnv(t, platform.Platform{}, r)
	err := NewPython().Apply(context.Background(), env)
	var ue *module.UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("Apply err = %v, want *module.UnsupportedError", err)
	}
	pythonAssertCommands(t, r, nil)
}
