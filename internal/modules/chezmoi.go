package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// chezmoiBackupLayout is the timestamp format of a backup directory name.
const chezmoiBackupLayout = "20060102-150405"

// chezmoiBackupDirPerm keeps backups private: they can hold ssh config and
// other files that only the user may read.
const chezmoiBackupDirPerm fs.FileMode = 0o700

// ChezmoiOutsideHomeError reports a path from `chezmoi managed` that is not
// under Env.Home, so it has no place in the backup directory.
type ChezmoiOutsideHomeError struct {
	Path string
	Home string
}

func (e *ChezmoiOutsideHomeError) Error() string {
	return fmt.Sprintf("chezmoi: managed path %q is outside home %q", e.Path, e.Home)
}

var (
	errChezmoiEmptyName    = errors.New("name must not be empty")
	errChezmoiInvalidEmail = errors.New("email must contain @")
)

type chezmoiModule struct {
	now func() time.Time
}

// NewChezmoi returns the module that links $HOME config files to the repo's
// chezmoi source. now stamps the backup directory; inject a fixed clock in
// tests.
func NewChezmoi(now func() time.Time) module.Module {
	return chezmoiModule{now: now}
}

func (chezmoiModule) ID() module.ID { return "chezmoi" }

func (chezmoiModule) Summary() string {
	return "Link $HOME config files to the repo with chezmoi (symlink mode)"
}

func (chezmoiModule) DependsOn() []module.ID { return []module.ID{"repo", "brew"} }

func (chezmoiModule) Supports(platform.Platform) bool { return true }

// Check reports Installed when chezmoi has a config file and its status
// lists no pending changes. Without a config it returns Missing and runs
// nothing, because the templates fail on the missing prompt data.
func (chezmoiModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	hasConfig, err := chezmoiHasConfig(env)
	if err != nil || !hasConfig {
		return module.StatusMissing, err
	}
	out, err := env.Run.Output(ctx, chezmoiCmd(env, "status"))
	if err != nil {
		return module.StatusMissing, fmt.Errorf("chezmoi: status: %w", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return module.StatusInstalled, nil
	}
	return module.StatusMissing, nil
}

// Apply initializes chezmoi when it has no config, shows the diff, and after
// confirmation backs up every managed regular file and applies the source
// state. A declined confirmation returns nil and changes nothing in $HOME.
func (m chezmoiModule) Apply(ctx context.Context, env module.Env) error {
	hasConfig, err := chezmoiHasConfig(env)
	if err != nil {
		return err
	}
	if !hasConfig {
		if err := chezmoiInit(ctx, env); err != nil {
			return err
		}
	}

	diff, err := env.Run.Output(ctx, chezmoiCmd(env, "diff"))
	if err != nil {
		return fmt.Errorf("chezmoi: diff: %w", err)
	}
	if len(bytes.TrimSpace(diff)) > 0 {
		ok, err := env.Confirm("Apply these changes to $HOME?", false)
		if err != nil {
			return fmt.Errorf("chezmoi: confirm apply: %w", err)
		}
		if !ok {
			return nil
		}
	}

	if err := m.backup(ctx, env); err != nil {
		return err
	}

	apply := chezmoiCmd(env, "apply")
	apply.Interactive = true
	if err := env.Run.Run(ctx, apply); err != nil {
		return fmt.Errorf("chezmoi: apply: %w", err)
	}
	return nil
}

// backup copies each managed target that exists as a regular file to
// <Home>/.dotfiles-backup/<timestamp>/<path relative to Home>.
func (m chezmoiModule) backup(ctx context.Context, env module.Env) error {
	out, err := env.Run.Output(ctx, chezmoiCmd(env, "managed", "--path-style", "absolute"))
	if err != nil {
		return fmt.Errorf("chezmoi: managed: %w", err)
	}
	root := filepath.Join(env.Home, ".dotfiles-backup", m.now().Format(chezmoiBackupLayout))
	for _, line := range strings.Split(string(out), "\n") {
		target := strings.TrimSpace(line)
		if target == "" {
			continue
		}
		if err := chezmoiBackupFile(env, root, target); err != nil {
			return err
		}
	}
	return nil
}

func chezmoiBackupFile(env module.Env, root, target string) error {
	// Lstat, not Stat: a target that is already a symlink into the repo
	// needs no backup.
	info, err := env.FS.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("chezmoi: backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	rel, err := filepath.Rel(env.Home, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return &ChezmoiOutsideHomeError{Path: target, Home: env.Home}
	}
	dst := filepath.Join(root, rel)
	if err := env.FS.MkdirAll(filepath.Dir(dst), chezmoiBackupDirPerm); err != nil {
		return fmt.Errorf("chezmoi: backup: %w", err)
	}
	data, err := env.FS.ReadFile(target)
	if err != nil {
		return fmt.Errorf("chezmoi: backup: %w", err)
	}
	if err := env.FS.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("chezmoi: backup: %w", err)
	}
	return nil
}

// chezmoiInit asks for the template data and runs a non-interactive init.
func chezmoiInit(ctx context.Context, env module.Env) error {
	name, err := env.Ask.Input("Your full name (for git and chezmoi)", prompt.InputOpts{
		Validate: chezmoiValidateName,
	})
	if err != nil {
		return fmt.Errorf("chezmoi: ask name: %w", err)
	}
	email, err := env.Ask.Input("Your email (for git and chezmoi)", prompt.InputOpts{
		Placeholder: "you@example.com",
		Validate:    chezmoiValidateEmail,
	})
	if err != nil {
		return fmt.Errorf("chezmoi: ask email: %w", err)
	}
	cmd := chezmoiCmd(env, "init",
		"--promptString", "name="+strings.TrimSpace(name),
		"--promptString", "email="+strings.TrimSpace(email),
	)
	if err := env.Run.Run(ctx, cmd); err != nil {
		return fmt.Errorf("chezmoi: init: %w", err)
	}
	return nil
}

func chezmoiValidateName(s string) error {
	if strings.TrimSpace(s) == "" {
		return errChezmoiEmptyName
	}
	return nil
}

func chezmoiValidateEmail(s string) error {
	if !strings.Contains(s, "@") {
		return errChezmoiInvalidEmail
	}
	return nil
}

func chezmoiHasConfig(env module.Env) (bool, error) {
	_, err := env.FS.Stat(chezmoiConfigPath(env))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("chezmoi: stat config: %w", err)
	}
	return true, nil
}

func chezmoiConfigPath(env module.Env) string {
	return filepath.Join(env.Home, ".config", "chezmoi", "chezmoi.toml")
}

// chezmoiCmd builds `chezmoi --source <RepoDir> <args...>`.
func chezmoiCmd(env module.Env, args ...string) runner.Cmd {
	return runner.Cmd{
		Name: "chezmoi",
		Args: append([]string{"--source", env.RepoDir}, args...),
	}
}
