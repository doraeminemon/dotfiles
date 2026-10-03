package modules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// repoCloneURL is the HTTPS clone URL. HTTPS works before an ssh key exists.
const repoCloneURL = "https://github.com/doraeminemon/dotfiles"

// repoOriginForms are the accepted origin URLs, after repoNormalizeURL.
var repoOriginForms = [...]string{
	repoCloneURL,
	"git@github.com:doraeminemon/dotfiles",
}

// RepoCLTMissingError reports that the Xcode Command Line Tools are not
// installed and the user stopped waiting for their installer.
type RepoCLTMissingError struct{}

func (*RepoCLTMissingError) Error() string {
	return "repo: Xcode Command Line Tools are not installed (xcode-select -p fails)"
}

// RepoOriginMismatchError reports a checkout at Dir whose origin is not this
// repo.
type RepoOriginMismatchError struct {
	Dir  string
	Got  string
	Want string
}

func (e *RepoOriginMismatchError) Error() string {
	return fmt.Sprintf("repo: %s has origin %q, want %q", e.Dir, e.Got, e.Want)
}

type repoModule struct{}

// NewRepo returns the module that ensures git (Xcode Command Line Tools on
// darwin) and clones this repo to Env.RepoDir.
func NewRepo() module.Module { return repoModule{} }

func (repoModule) ID() module.ID { return "repo" }

func (repoModule) Summary() string {
	return "Ensure git and clone the dotfiles repo"
}

// DependsOn lists apt, which installs git on linux. Registry.Plan skips it on
// darwin.
func (repoModule) DependsOn() []module.ID { return []module.ID{"apt"} }

func (repoModule) Supports(p platform.Platform) bool {
	switch p.OS {
	case platform.Darwin, platform.Linux:
		return true
	default:
		return false
	}
}

func (repoModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	if env.Platform.OS == platform.Darwin && !repoHasCLT(ctx, env) {
		return module.StatusMissing, nil
	}
	cloned, err := repoIsCloned(env)
	if err != nil {
		return module.StatusMissing, err
	}
	if !cloned {
		return module.StatusMissing, nil
	}
	if err := repoCheckOrigin(ctx, env); err != nil {
		return module.StatusMissing, err
	}
	return module.StatusInstalled, nil
}

func (repoModule) Apply(ctx context.Context, env module.Env) error {
	if env.Platform.OS == platform.Darwin {
		if err := repoEnsureCLT(ctx, env); err != nil {
			return err
		}
	}
	cloned, err := repoIsCloned(env)
	if err != nil {
		return err
	}
	if cloned {
		return repoCheckOrigin(ctx, env)
	}
	parent := filepath.Dir(env.RepoDir)
	if err := env.FS.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("repo: create %s: %w", parent, err)
	}
	clone := runner.Cmd{Name: "git", Args: []string{"clone", repoCloneURL, env.RepoDir}}
	if err := env.Run.Run(ctx, clone); err != nil {
		return fmt.Errorf("repo: clone into %s: %w", env.RepoDir, err)
	}
	return nil
}

// repoXcodeSelectPath is `xcode-select -p`, which fails while the Command
// Line Tools are missing.
var repoXcodeSelectPath = runner.Cmd{Name: "xcode-select", Args: []string{"-p"}}

func repoHasCLT(ctx context.Context, env module.Env) bool {
	_, err := env.Run.Output(ctx, repoXcodeSelectPath)
	return err == nil
}

// repoEnsureCLT starts the Command Line Tools installer when they are missing
// and waits until the user confirms that it finished.
func repoEnsureCLT(ctx context.Context, env module.Env) error {
	if repoHasCLT(ctx, env) {
		return nil
	}
	install := runner.Cmd{Name: "xcode-select", Args: []string{"--install"}, Interactive: true}
	if err := env.Run.Run(ctx, install); err != nil {
		return fmt.Errorf("repo: start Xcode Command Line Tools install: %w", err)
	}
	for {
		done, err := env.Ask.Confirm("Xcode Command Line Tools finished installing?", true)
		if err != nil {
			return fmt.Errorf("repo: wait for Xcode Command Line Tools: %w", err)
		}
		if !done {
			return &RepoCLTMissingError{}
		}
		if repoHasCLT(ctx, env) {
			return nil
		}
	}
}

// repoIsCloned reports whether RepoDir holds a .git entry.
func repoIsCloned(env module.Env) (bool, error) {
	gitDir := filepath.Join(env.RepoDir, ".git")
	_, err := env.FS.Stat(gitDir)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("repo: stat %s: %w", gitDir, err)
	}
}

// repoCheckOrigin returns *RepoOriginMismatchError when the checkout at
// RepoDir has a non-empty origin that is not this repo. A checkout with no
// origin (the command exits non-zero) or an empty one (as under --dry-run)
// is a local checkout and passes.
func repoCheckOrigin(ctx context.Context, env module.Env) error {
	getURL := runner.Cmd{Name: "git", Args: []string{"-C", env.RepoDir, "remote", "get-url", "origin"}}
	out, err := env.Run.Output(ctx, getURL)
	var cmdErr *runner.CommandError
	if errors.As(err, &cmdErr) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("repo: read origin of %s: %w", env.RepoDir, err)
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		return nil
	}
	norm := repoNormalizeURL(got)
	for _, want := range repoOriginForms {
		if norm == want {
			return nil
		}
	}
	return &RepoOriginMismatchError{Dir: env.RepoDir, Got: got, Want: repoCloneURL}
}

// repoNormalizeURL drops a trailing slash and .git suffix.
func repoNormalizeURL(u string) string {
	u = strings.TrimSuffix(u, "/")
	return strings.TrimSuffix(u, ".git")
}
