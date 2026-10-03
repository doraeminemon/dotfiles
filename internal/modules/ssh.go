package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// errSSHEmail rejects an SSH key comment that is not an email address.
var errSSHEmail = errors.New("enter an email address")

// SSHPublicKeyError reports a public key file that does not have the
// "<type> <base64> [comment]" form.
type SSHPublicKeyError struct {
	Path string
}

func (e *SSHPublicKeyError) Error() string {
	return fmt.Sprintf("ssh: %s is not an OpenSSH public key", e.Path)
}

// sshModule creates an ed25519 key, adds it to the agent, and uploads the
// public key to GitHub with gh. It never reads the private key.
type sshModule struct{}

// NewSSH returns the ssh module.
func NewSSH() module.Module { return sshModule{} }

func (sshModule) ID() module.ID { return "ssh" }

func (sshModule) Summary() string {
	return "ed25519 SSH key, ssh-agent, and the key uploaded to GitHub"
}

func (sshModule) DependsOn() []module.ID { return []module.ID{"brew", "chezmoi"} }

func (sshModule) Supports(p platform.Platform) bool {
	return p.OS == platform.Darwin || p.OS == platform.Linux
}

// sshKeyPath is the private key path. Only its existence is ever checked.
func sshKeyPath(env module.Env) string {
	return filepath.Join(env.Home, ".ssh", "id_ed25519")
}

func sshPubPath(env module.Env) string { return sshKeyPath(env) + ".pub" }

func (sshModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	for _, p := range []string{sshKeyPath(env), sshPubPath(env)} {
		ok, err := sshExists(env, p)
		if err != nil || !ok {
			return module.StatusMissing, err
		}
	}
	authed, err := sshGHAuthed(ctx, env)
	if err != nil || !authed {
		return module.StatusMissing, err
	}
	listed, err := sshUploaded(ctx, env)
	if err != nil || listed != sshKeyListed {
		return module.StatusMissing, err
	}
	return module.StatusInstalled, nil
}

func (sshModule) Apply(ctx context.Context, env module.Env) error {
	key := sshKeyPath(env)
	ok, err := sshExists(env, key)
	if err != nil {
		return err
	}
	if !ok {
		if err := sshGenerate(ctx, env, key); err != nil {
			return err
		}
	}

	authed, err := sshGHAuthed(ctx, env)
	if err != nil {
		return err
	}
	if !authed {
		yes, err := env.Confirm("Log in to GitHub with gh?", true)
		if err != nil {
			return fmt.Errorf("ssh: %w", err)
		}
		if !yes {
			return nil
		}
		login := runner.Cmd{
			Name:        "gh",
			Args:        []string{"auth", "login", "--git-protocol", "ssh", "--web"},
			Interactive: true,
		}
		if err := env.Run.Run(ctx, login); err != nil {
			return fmt.Errorf("ssh: gh auth login: %w", err)
		}
	}

	listed, err := sshUploaded(ctx, env)
	if err != nil || listed == sshKeyListed {
		return err
	}
	yes, err := env.Confirm("Upload the public key to GitHub?", true)
	if err != nil {
		return fmt.Errorf("ssh: %w", err)
	}
	if !yes {
		return nil
	}
	if listed == sshKeyListFailed {
		if err := sshRefreshScope(ctx, env); err != nil {
			return err
		}
	}
	return sshUpload(ctx, env)
}

// sshRefreshScope asks gh, in the terminal, for the admin:public_key scope
// that listing and adding ssh keys need.
func sshRefreshScope(ctx context.Context, env module.Env) error {
	refresh := runner.Cmd{
		Name:        "gh",
		Args:        []string{"auth", "refresh", "-h", "github.com", "-s", "admin:public_key"},
		Interactive: true,
	}
	if err := env.Run.Run(ctx, refresh); err != nil {
		return fmt.Errorf("ssh: gh auth refresh: %w", err)
	}
	return nil
}

// sshGenerate asks for the key comment, runs ssh-keygen in the terminal so it
// can prompt for the passphrase, and adds the new key to the agent.
func sshGenerate(ctx context.Context, env module.Env, key string) error {
	email, err := env.Ask.Input("Email for the SSH key comment", prompt.InputOpts{
		Placeholder: "you@example.com",
		Validate: func(s string) error {
			if !strings.Contains(s, "@") {
				return errSSHEmail
			}
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("ssh: %w", err)
	}
	if err := env.FS.MkdirAll(filepath.Dir(key), 0o700); err != nil {
		return fmt.Errorf("ssh: %w", err)
	}
	keygen := runner.Cmd{
		Name:        "ssh-keygen",
		Args:        []string{"-t", "ed25519", "-C", email, "-f", key},
		Interactive: true,
	}
	if err := env.Run.Run(ctx, keygen); err != nil {
		return fmt.Errorf("ssh: ssh-keygen: %w", err)
	}
	args := []string{key}
	if env.Platform.OS == platform.Darwin {
		args = []string{"--apple-use-keychain", key}
	}
	if err := env.Run.Run(ctx, runner.Cmd{Name: "ssh-add", Args: args, Interactive: true}); err != nil {
		return fmt.Errorf("ssh: ssh-add: %w", err)
	}
	return nil
}

// sshUpload adds the public key to GitHub, titled with the short hostname.
func sshUpload(ctx context.Context, env module.Env) error {
	out, err := env.Run.Output(ctx, runner.Cmd{Name: "hostname", Args: []string{"-s"}})
	if err != nil {
		return fmt.Errorf("ssh: hostname: %w", err)
	}
	args := []string{"ssh-key", "add", sshPubPath(env)}
	// An empty hostname (as under --dry-run) leaves the title to GitHub.
	if host := strings.TrimSpace(string(out)); host != "" {
		args = append(args, "--title", host)
	}
	if err := env.Run.Run(ctx, runner.Cmd{Name: "gh", Args: args}); err != nil {
		return fmt.Errorf("ssh: gh ssh-key add: %w", err)
	}
	return nil
}

// sshGHAuthed reports whether gh is logged in. A failing or absent gh means
// not logged in.
func sshGHAuthed(ctx context.Context, env module.Env) (bool, error) {
	_, err := env.Run.Output(ctx, runner.Cmd{Name: "gh", Args: []string{"auth", "status"}})
	if err == nil {
		return true, nil
	}
	var cmdErr *runner.CommandError
	var nfErr *runner.NotFoundError
	if errors.As(err, &cmdErr) || errors.As(err, &nfErr) {
		return false, nil
	}
	return false, fmt.Errorf("ssh: gh auth status: %w", err)
}

// sshKeyListing is the result of looking for the local public key on GitHub.
type sshKeyListing int

const (
	// sshKeyNotListed: the list succeeded without the key, or there is no
	// public key file yet.
	sshKeyNotListed sshKeyListing = iota
	// sshKeyListed: GitHub lists the key.
	sshKeyListed
	// sshKeyListFailed: `gh ssh-key list` exited non-zero, for example with
	// HTTP 404 when the token lacks the admin:public_key scope.
	sshKeyListFailed
)

// sshUploaded looks for the local public key in `gh ssh-key list`. A missing
// public key file (as after a dry-run ssh-keygen) is not listed.
func sshUploaded(ctx context.Context, env module.Env) (sshKeyListing, error) {
	body, err := sshPubBody(env)
	if errors.Is(err, fs.ErrNotExist) {
		return sshKeyNotListed, nil
	}
	if err != nil {
		return sshKeyNotListed, err
	}
	out, err := env.Run.Output(ctx, runner.Cmd{Name: "gh", Args: []string{"ssh-key", "list"}})
	var cmdErr *runner.CommandError
	if errors.As(err, &cmdErr) {
		return sshKeyListFailed, nil
	}
	if err != nil {
		return sshKeyNotListed, fmt.Errorf("ssh: gh ssh-key list: %w", err)
	}
	if bytes.Contains(out, body) {
		return sshKeyListed, nil
	}
	return sshKeyNotListed, nil
}

// sshPubBody returns the base64 field of the public key file.
func sshPubBody(env module.Env) ([]byte, error) {
	path := sshPubPath(env)
	data, err := env.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: %w", err)
	}
	fields := bytes.Fields(data)
	if len(fields) < 2 {
		return nil, &SSHPublicKeyError{Path: path}
	}
	return fields[1], nil
}

func sshExists(env module.Env, path string) (bool, error) {
	_, err := env.FS.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("ssh: %w", err)
}
