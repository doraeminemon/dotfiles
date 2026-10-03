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
	sshTestHome = "/home/u"
	sshTestKey  = "/home/u/.ssh/id_ed25519"
	sshTestPub  = "/home/u/.ssh/id_ed25519.pub"
	// sshTestBody is a made-up base64 field, not a real key.
	sshTestBody = "AAAAC3NzaC1lZDI1NTE5AAAAIFAKEFAKEFAKEFAKE"
)

var (
	sshTestDarwin = platform.Platform{OS: platform.Darwin, Arch: platform.ARM64}
	sshTestLinux  = platform.Platform{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt}
	sshTestFail   = runner.Result{Err: &runner.CommandError{ExitCode: 1}}
)

type sshTestEnv struct {
	env module.Env
	run *runner.FakeRunner
	ask *prompt.FakePrompter
	fs  *module.MemFS
}

func newSSHTestEnv(t *testing.T, p platform.Platform, withKey bool) sshTestEnv {
	t.Helper()
	te := sshTestEnv{
		run: &runner.FakeRunner{Script: map[string]runner.Result{}},
		ask: &prompt.FakePrompter{},
		fs:  &module.MemFS{},
	}
	if withKey {
		if err := te.fs.MkdirAll("/home/u/.ssh", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := te.fs.WriteFile(sshTestKey, []byte("placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
		pub := "ssh-ed25519 " + sshTestBody + " me@example.com\n"
		if err := te.fs.WriteFile(sshTestPub, []byte(pub), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	te.env = module.Env{Platform: p, Run: te.run, Ask: te.ask, FS: te.fs, Home: sshTestHome}
	return te
}

func (te sshTestEnv) uploaded() {
	te.run.Script["gh ssh-key list"] = runner.Result{
		Stdout: []byte("laptop\tssh-ed25519 " + sshTestBody + "\t2026-01-01\t1\tauthentication\n"),
	}
}

func sshAssertCommands(t *testing.T, run *runner.FakeRunner, want []string) {
	t.Helper()
	if got := run.Commands(); !slices.Equal(got, want) {
		t.Fatalf("commands:\n got  %q\n want %q", got, want)
	}
}

func TestSSHMetadata(t *testing.T) {
	m := NewSSH()
	if m.ID() != "ssh" {
		t.Errorf("ID = %q", m.ID())
	}
	if got := m.DependsOn(); !slices.Equal(got, []module.ID{"brew", "chezmoi"}) {
		t.Errorf("DependsOn = %v", got)
	}
	if !m.Supports(sshTestDarwin) || !m.Supports(sshTestLinux) {
		t.Error("Supports darwin and linux = false")
	}
}

func TestSSHCheck(t *testing.T) {
	tests := []struct {
		name    string
		withKey bool
		script  func(sshTestEnv)
		want    module.Status
		cmds    []string
	}{
		{name: "no key", want: module.StatusMissing, cmds: []string{}},
		{
			name: "not logged in", withKey: true,
			script: func(te sshTestEnv) { te.run.Script["gh auth status"] = sshTestFail },
			want:   module.StatusMissing,
			cmds:   []string{"gh auth status"},
		},
		{
			name: "not uploaded", withKey: true,
			want: module.StatusMissing,
			cmds: []string{"gh auth status", "gh ssh-key list"},
		},
		{
			name: "installed", withKey: true,
			script: func(te sshTestEnv) { te.uploaded() },
			want:   module.StatusInstalled,
			cmds:   []string{"gh auth status", "gh ssh-key list"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newSSHTestEnv(t, sshTestDarwin, tt.withKey)
			if tt.script != nil {
				tt.script(te)
			}
			got, err := NewSSH().Check(context.Background(), te.env)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Check = %v, want %v", got, tt.want)
			}
			sshAssertCommands(t, te.run, tt.cmds)
		})
	}
}

func TestSSHApplyNewKeyDarwin(t *testing.T) {
	te := newSSHTestEnv(t, sshTestDarwin, false)
	te.ask.Inputs = []string{"me@example.com"}
	te.ask.Confirms = []bool{true, true}
	te.run.Script["gh auth status"] = sshTestFail
	te.run.Script["hostname -s"] = runner.Result{Stdout: []byte("mac\n")}

	if err := NewSSH().Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	sshAssertCommands(t, te.run, []string{
		"ssh-keygen -t ed25519 -C me@example.com -f " + sshTestKey,
		"ssh-add --apple-use-keychain " + sshTestKey,
		"gh auth status",
		"gh auth login --git-protocol ssh --web",
		"hostname -s",
		"gh ssh-key add " + sshTestPub + " --title mac",
	})
	for _, c := range te.run.Calls {
		want := c.Name == "ssh-keygen" || c.Name == "ssh-add" || c.String() == "gh auth login --git-protocol ssh --web"
		if c.Interactive != want {
			t.Errorf("%s: Interactive = %v, want %v", c, c.Interactive, want)
		}
	}
	info, err := te.fs.Stat("/home/u/.ssh")
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf(".ssh mode = %o, want 700", perm)
	}
}

func TestSSHApplyNewKeyLinux(t *testing.T) {
	te := newSSHTestEnv(t, sshTestLinux, false)
	te.ask.Inputs = []string{"me@example.com"}
	te.ask.Confirms = []bool{false}

	if err := NewSSH().Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	sshAssertCommands(t, te.run, []string{
		"ssh-keygen -t ed25519 -C me@example.com -f " + sshTestKey,
		"ssh-add " + sshTestKey,
		"gh auth status",
	})
}

func TestSSHApplyExistingKeyDeclineUpload(t *testing.T) {
	for _, p := range []platform.Platform{sshTestDarwin, sshTestLinux} {
		t.Run(p.String(), func(t *testing.T) {
			te := newSSHTestEnv(t, p, true)
			te.ask.Confirms = []bool{false}

			if err := NewSSH().Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			sshAssertCommands(t, te.run, []string{"gh auth status", "gh ssh-key list"})
			if !slices.Equal(te.ask.Asked, []string{"Upload the public key to GitHub?"}) {
				t.Errorf("Asked = %q", te.ask.Asked)
			}
		})
	}
}

func TestSSHApplyExistingKeyUpload(t *testing.T) {
	te := newSSHTestEnv(t, sshTestLinux, true)
	te.env.Yes = true
	te.run.Script["hostname -s"] = runner.Result{Stdout: []byte("box\n")}

	if err := NewSSH().Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	sshAssertCommands(t, te.run, []string{
		"gh auth status",
		"gh ssh-key list",
		"hostname -s",
		"gh ssh-key add " + sshTestPub + " --title box",
	})
}

func TestSSHApplyIdempotent(t *testing.T) {
	te := newSSHTestEnv(t, sshTestDarwin, true)
	te.uploaded()

	if err := NewSSH().Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	sshAssertCommands(t, te.run, []string{"gh auth status", "gh ssh-key list"})
	if len(te.ask.Asked) != 0 {
		t.Errorf("Asked = %q", te.ask.Asked)
	}
}

func TestSSHApplyDeclineLogin(t *testing.T) {
	te := newSSHTestEnv(t, sshTestLinux, true)
	te.ask.Confirms = []bool{false}
	te.run.Script["gh auth status"] = sshTestFail

	if err := NewSSH().Apply(context.Background(), te.env); err != nil {
		t.Fatal(err)
	}
	sshAssertCommands(t, te.run, []string{"gh auth status"})
}

func TestSSHApplyRejectsBadEmail(t *testing.T) {
	te := newSSHTestEnv(t, sshTestDarwin, false)
	te.ask.Inputs = []string{"not-an-email"}

	err := NewSSH().Apply(context.Background(), te.env)
	if !errors.Is(err, errSSHEmail) {
		t.Fatalf("err = %v, want errSSHEmail", err)
	}
	sshAssertCommands(t, te.run, []string{})
}

func TestSSHCheckMalformedPub(t *testing.T) {
	te := newSSHTestEnv(t, sshTestDarwin, true)
	if err := te.fs.WriteFile(sshTestPub, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewSSH().Check(context.Background(), te.env)
	var pkErr *SSHPublicKeyError
	if !errors.As(err, &pkErr) || pkErr.Path != sshTestPub {
		t.Fatalf("err = %v, want *SSHPublicKeyError for %s", err, sshTestPub)
	}
	sshAssertCommands(t, te.run, []string{"gh auth status"})
}

// sshTestRefresh is the scope refresh that runs when the key list failed.
const sshTestRefresh = "gh auth refresh -h github.com -s admin:public_key"

func TestSSHCheckListFails(t *testing.T) {
	for _, p := range []platform.Platform{sshTestDarwin, sshTestLinux} {
		t.Run(p.String(), func(t *testing.T) {
			te := newSSHTestEnv(t, p, true)
			te.run.Script["gh ssh-key list"] = sshTestFail

			got, err := NewSSH().Check(context.Background(), te.env)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got != module.StatusMissing {
				t.Errorf("Check = %v, want %v", got, module.StatusMissing)
			}
			sshAssertCommands(t, te.run, []string{"gh auth status", "gh ssh-key list"})
		})
	}
}

func TestSSHApplyListFailsRefreshesScope(t *testing.T) {
	for _, p := range []platform.Platform{sshTestDarwin, sshTestLinux} {
		t.Run(p.String(), func(t *testing.T) {
			te := newSSHTestEnv(t, p, true)
			te.ask.Confirms = []bool{true}
			te.run.Script["gh ssh-key list"] = sshTestFail
			te.run.Script["hostname -s"] = runner.Result{Stdout: []byte("box\n")}

			if err := NewSSH().Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			sshAssertCommands(t, te.run, []string{
				"gh auth status",
				"gh ssh-key list",
				sshTestRefresh,
				"hostname -s",
				"gh ssh-key add " + sshTestPub + " --title box",
			})
			for _, c := range te.run.Calls {
				if want := c.String() == sshTestRefresh; c.Interactive != want {
					t.Errorf("%s: Interactive = %v, want %v", c, c.Interactive, want)
				}
			}
		})
	}
}

func TestSSHApplyListFailsDeclineUpload(t *testing.T) {
	for _, p := range []platform.Platform{sshTestDarwin, sshTestLinux} {
		t.Run(p.String(), func(t *testing.T) {
			te := newSSHTestEnv(t, p, true)
			te.ask.Confirms = []bool{false}
			te.run.Script["gh ssh-key list"] = sshTestFail

			if err := NewSSH().Apply(context.Background(), te.env); err != nil {
				t.Fatal(err)
			}
			sshAssertCommands(t, te.run, []string{"gh auth status", "gh ssh-key list"})
		})
	}
}

func TestSSHApplyRefreshFails(t *testing.T) {
	te := newSSHTestEnv(t, sshTestLinux, true)
	te.env.Yes = true
	te.run.Script["gh ssh-key list"] = sshTestFail
	te.run.Script[sshTestRefresh] = sshTestFail

	err := NewSSH().Apply(context.Background(), te.env)
	var cmdErr *runner.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v, want *runner.CommandError", err)
	}
	sshAssertCommands(t, te.run, []string{"gh auth status", "gh ssh-key list", sshTestRefresh})
}
