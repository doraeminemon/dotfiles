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

var (
	repoTestDarwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	repoTestLinux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
)

const (
	repoTestDarwinDir = "/Users/u/Projects/dotfiles"
	repoTestLinuxDir  = "/home/u/Projects/dotfiles"

	repoTestXcodeP       = "xcode-select -p"
	repoTestXcodeInstall = "xcode-select --install"
)

func repoTestGetURL(dir string) string { return "git -C " + dir + " remote get-url origin" }

func repoTestClone(dir string) string { return "git clone " + repoCloneURL + " " + dir }

func repoTestFail(name string) runner.Result {
	return runner.Result{Err: &runner.CommandError{Cmd: runner.Cmd{Name: name}, ExitCode: 2}}
}

func repoTestOK(stdout string) runner.Result { return runner.Result{Stdout: []byte(stdout)} }

// repoSeqRunner records every call in a FakeRunner, then answers a command
// from its queue in seq while the queue is not empty. It scripts a command
// that fails first and succeeds later, such as xcode-select -p.
type repoSeqRunner struct {
	fake *runner.FakeRunner
	seq  map[string][]runner.Result
}

func (r *repoSeqRunner) next(c runner.Cmd, fallback runner.Result) runner.Result {
	q := r.seq[c.String()]
	if len(q) == 0 {
		return fallback
	}
	r.seq[c.String()] = q[1:]
	return q[0]
}

func (r *repoSeqRunner) Run(ctx context.Context, c runner.Cmd) error {
	err := r.fake.Run(ctx, c)
	return r.next(c, runner.Result{Err: err}).Err
}

func (r *repoSeqRunner) Output(ctx context.Context, c runner.Cmd) ([]byte, error) {
	out, err := r.fake.Output(ctx, c)
	res := r.next(c, runner.Result{Stdout: out, Err: err})
	return res.Stdout, res.Err
}

type repoTestEnv struct {
	env  module.Env
	fake *runner.FakeRunner
	ask  *prompt.FakePrompter
	fs   *module.MemFS
}

func newRepoTestEnv(t *testing.T, p platform.Platform, dir string, cloned bool, seq map[string][]runner.Result, confirms []bool) repoTestEnv {
	t.Helper()
	memFS := &module.MemFS{}
	if cloned {
		if err := memFS.MkdirAll(dir+"/.git", 0o755); err != nil {
			t.Fatalf("seed .git: %v", err)
		}
	}
	fake := &runner.FakeRunner{}
	if seq == nil {
		seq = map[string][]runner.Result{}
	}
	ask := &prompt.FakePrompter{Confirms: confirms}
	return repoTestEnv{
		env: module.Env{
			Platform: p,
			Run:      &repoSeqRunner{fake: fake, seq: seq},
			Ask:      ask,
			FS:       memFS,
			RepoDir:  dir,
			Home:     "/home/u",
		},
		fake: fake,
		ask:  ask,
		fs:   memFS,
	}
}

func TestRepoMetadata(t *testing.T) {
	m := NewRepo()
	if m.ID() != "repo" {
		t.Errorf("ID = %q, want repo", m.ID())
	}
	if got := m.DependsOn(); !slices.Equal(got, []module.ID{"apt"}) {
		t.Errorf("DependsOn = %v, want [apt]", got)
	}
	for _, p := range []platform.Platform{repoTestDarwin, repoTestLinux} {
		if !m.Supports(p) {
			t.Errorf("Supports(%v) = false, want true", p)
		}
	}
	if m.Supports(platform.Platform{}) {
		t.Error("Supports(zero Platform) = true, want false")
	}
}

func TestRepoCheck(t *testing.T) {
	tests := []struct {
		name     string
		p        platform.Platform
		dir      string
		cloned   bool
		seq      map[string][]runner.Result
		want     module.Status
		wantErr  bool
		wantCmds []string
	}{
		{
			name:     "darwin without CLT",
			p:        repoTestDarwin,
			dir:      repoTestDarwinDir,
			cloned:   true,
			seq:      map[string][]runner.Result{repoTestXcodeP: {repoTestFail("xcode-select")}},
			want:     module.StatusMissing,
			wantCmds: []string{repoTestXcodeP},
		},
		{
			name:     "darwin with CLT, no checkout",
			p:        repoTestDarwin,
			dir:      repoTestDarwinDir,
			want:     module.StatusMissing,
			wantCmds: []string{repoTestXcodeP},
		},
		{
			name:   "darwin with CLT and https checkout",
			p:      repoTestDarwin,
			dir:    repoTestDarwinDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestDarwinDir): {repoTestOK(repoCloneURL + "\n")},
			},
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestXcodeP, repoTestGetURL(repoTestDarwinDir)},
		},
		{
			name:     "linux, no checkout",
			p:        repoTestLinux,
			dir:      repoTestLinuxDir,
			want:     module.StatusMissing,
			wantCmds: []string{},
		},
		{
			name:   "linux ssh origin",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestOK("git@github.com:doraeminemon/dotfiles.git\n")},
			},
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
		},
		{
			name:   "linux https origin with .git",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestOK(repoCloneURL + ".git\n")},
			},
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
		},
		{
			name:   "linux other origin",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestOK("https://github.com/someone/dotfiles\n")},
			},
			want:     module.StatusMissing,
			wantErr:  true,
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
		},
		{
			name:   "darwin local checkout without origin",
			p:      repoTestDarwin,
			dir:    repoTestDarwinDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestDarwinDir): {repoTestFail("git")},
			},
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestXcodeP, repoTestGetURL(repoTestDarwinDir)},
		},
		{
			name:   "linux local checkout without origin",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestFail("git")},
			},
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
		},
		{
			name:   "darwin other origin",
			p:      repoTestDarwin,
			dir:    repoTestDarwinDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestDarwinDir): {repoTestOK("git@github.com:someone/dotfiles.git\n")},
			},
			want:     module.StatusMissing,
			wantErr:  true,
			wantCmds: []string{repoTestXcodeP, repoTestGetURL(repoTestDarwinDir)},
		},
		{
			name:     "linux empty origin output",
			p:        repoTestLinux,
			dir:      repoTestLinuxDir,
			cloned:   true,
			want:     module.StatusInstalled,
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newRepoTestEnv(t, tt.p, tt.dir, tt.cloned, tt.seq, nil)
			got, err := NewRepo().Check(context.Background(), te.env)
			if tt.wantErr {
				var mismatch *RepoOriginMismatchError
				if !errors.As(err, &mismatch) {
					t.Fatalf("err = %v, want *RepoOriginMismatchError", err)
				}
			} else if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got != tt.want {
				t.Errorf("status = %v, want %v", got, tt.want)
			}
			if cmds := te.fake.Commands(); !slices.Equal(cmds, tt.wantCmds) {
				t.Errorf("commands = %q, want %q", cmds, tt.wantCmds)
			}
		})
	}
}

func TestRepoApply(t *testing.T) {
	tests := []struct {
		name      string
		p         platform.Platform
		dir       string
		cloned    bool
		seq       map[string][]runner.Result
		confirms  []bool
		wantErr   func(error) bool
		wantCmds  []string
		wantAsked int
		wantDir   bool
	}{
		{
			name: "darwin without CLT waits, then clones",
			p:    repoTestDarwin,
			dir:  repoTestDarwinDir,
			seq: map[string][]runner.Result{
				repoTestXcodeP: {repoTestFail("xcode-select"), repoTestFail("xcode-select"), repoTestOK("/Library/Developer/CommandLineTools\n")},
			},
			confirms: []bool{true, true},
			wantCmds: []string{
				repoTestXcodeP, repoTestXcodeInstall, repoTestXcodeP, repoTestXcodeP,
				repoTestClone(repoTestDarwinDir),
			},
			wantAsked: 2,
			wantDir:   true,
		},
		{
			name:     "darwin without CLT, user gives up",
			p:        repoTestDarwin,
			dir:      repoTestDarwinDir,
			seq:      map[string][]runner.Result{repoTestXcodeP: {repoTestFail("xcode-select")}},
			confirms: []bool{false},
			wantErr: func(err error) bool {
				var missing *RepoCLTMissingError
				return errors.As(err, &missing)
			},
			wantCmds:  []string{repoTestXcodeP, repoTestXcodeInstall},
			wantAsked: 1,
		},
		{
			name:   "darwin existing checkout, no clone",
			p:      repoTestDarwin,
			dir:    repoTestDarwinDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestDarwinDir): {repoTestOK(repoCloneURL + "\n")},
			},
			wantCmds: []string{repoTestXcodeP, repoTestGetURL(repoTestDarwinDir)},
			wantDir:  true,
		},
		{
			name:     "linux clones without xcode-select",
			p:        repoTestLinux,
			dir:      repoTestLinuxDir,
			wantCmds: []string{repoTestClone(repoTestLinuxDir)},
			wantDir:  true,
		},
		{
			name:   "linux existing checkout, no clone",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestOK("git@github.com:doraeminemon/dotfiles.git\n")},
			},
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
			wantDir:  true,
		},
		{
			name:   "linux checkout of another repo",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestOK("https://github.com/someone/dotfiles\n")},
			},
			wantErr: func(err error) bool {
				var mismatch *RepoOriginMismatchError
				return errors.As(err, &mismatch) &&
					mismatch.Dir == repoTestLinuxDir &&
					mismatch.Got == "https://github.com/someone/dotfiles" &&
					mismatch.Want == repoCloneURL
			},
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
			wantDir:  true,
		},
		{
			name:   "darwin local checkout without origin, no clone",
			p:      repoTestDarwin,
			dir:    repoTestDarwinDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestDarwinDir): {repoTestFail("git")},
			},
			wantCmds: []string{repoTestXcodeP, repoTestGetURL(repoTestDarwinDir)},
			wantDir:  true,
		},
		{
			name:   "linux local checkout without origin, no clone",
			p:      repoTestLinux,
			dir:    repoTestLinuxDir,
			cloned: true,
			seq: map[string][]runner.Result{
				repoTestGetURL(repoTestLinuxDir): {repoTestFail("git")},
			},
			wantCmds: []string{repoTestGetURL(repoTestLinuxDir)},
			wantDir:  true,
		},
		{
			name: "linux clone fails",
			p:    repoTestLinux,
			dir:  repoTestLinuxDir,
			seq: map[string][]runner.Result{
				repoTestClone(repoTestLinuxDir): {repoTestFail("git")},
			},
			wantErr: func(err error) bool {
				var cmdErr *runner.CommandError
				return errors.As(err, &cmdErr)
			},
			wantCmds: []string{repoTestClone(repoTestLinuxDir)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newRepoTestEnv(t, tt.p, tt.dir, tt.cloned, tt.seq, tt.confirms)
			err := NewRepo().Apply(context.Background(), te.env)
			if tt.wantErr != nil {
				if !tt.wantErr(err) {
					t.Fatalf("err = %v, unexpected", err)
				}
			} else if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if cmds := te.fake.Commands(); !slices.Equal(cmds, tt.wantCmds) {
				t.Errorf("commands = %q, want %q", cmds, tt.wantCmds)
			}
			if got := len(te.ask.Asked); got != tt.wantAsked {
				t.Errorf("asked %d questions (%q), want %d", got, te.ask.Asked, tt.wantAsked)
			}
			for _, c := range te.fake.Calls {
				if wantInteractive := c.String() == repoTestXcodeInstall; c.Interactive != wantInteractive {
					t.Errorf("%s: Interactive = %v, want %v", c, c.Interactive, wantInteractive)
				}
			}
			// The clone (scripted to succeed) creates no files, so only the
			// parent directory, made by MkdirAll, shows up for a fresh clone.
			if tt.wantDir && !tt.cloned {
				if _, err := te.fs.Stat(filepath.Dir(tt.dir)); err != nil {
					t.Errorf("parent of %s not created: %v", tt.dir, err)
				}
			}
		})
	}
}
