package modules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const fishShellsFile = "/etc/shells"

// fishParseError reports command output that does not have the expected shape.
type fishParseError struct {
	What   string
	Output string
}

func (e *fishParseError) Error() string {
	return fmt.Sprintf("fish: cannot parse %s from %q", e.What, e.Output)
}

type fishModule struct{}

// NewFish returns the module that makes fish a login shell and runs fisher.
func NewFish() module.Module { return fishModule{} }

func (fishModule) ID() module.ID { return "fish" }

func (fishModule) Summary() string {
	return "Register fish in /etc/shells, set it as login shell, run fisher update"
}

func (fishModule) DependsOn() []module.ID { return []module.ID{"brew", "chezmoi"} }

func (fishModule) Supports(p platform.Platform) bool {
	return p.OS == platform.Darwin || p.OS == platform.Linux
}

// fishPath returns the absolute path of the fish binary.
func fishPath(ctx context.Context, env module.Env) (string, error) {
	out, err := env.Run.Output(ctx, runner.Cmd{Name: "sh", Args: []string{"-c", "command -v fish"}})
	if err != nil {
		return "", fmt.Errorf("fish: locate fish: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", &fishParseError{What: "fish path", Output: string(out)}
	}
	return path, nil
}

// fishInShells reports whether path is a line of /etc/shells.
func fishInShells(env module.Env, path string) (bool, error) {
	data, err := env.FS.ReadFile(fishShellsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("fish: read %s: %w", fishShellsFile, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == path {
			return true, nil
		}
	}
	return false, nil
}

// fishUser returns the name of the current user.
func fishUser(ctx context.Context, env module.Env) (string, error) {
	out, err := env.Run.Output(ctx, runner.Cmd{Name: "id", Args: []string{"-un"}})
	if err != nil {
		return "", fmt.Errorf("fish: current user: %w", err)
	}
	user := strings.TrimSpace(string(out))
	if user == "" {
		return "", &fishParseError{What: "user name", Output: string(out)}
	}
	return user, nil
}

// fishLoginShell returns the login shell of user.
func fishLoginShell(ctx context.Context, env module.Env, user string) (string, error) {
	if env.Platform.OS == platform.Darwin {
		out, err := env.Run.Output(ctx, runner.Cmd{Name: "dscl", Args: []string{".", "-read", "/Users/" + user, "UserShell"}})
		if err != nil {
			return "", fmt.Errorf("fish: read login shell: %w", err)
		}
		field, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "UserShell:")
		shell := strings.TrimSpace(field)
		if !ok || shell == "" {
			return "", &fishParseError{What: "UserShell", Output: string(out)}
		}
		return shell, nil
	}
	out, err := env.Run.Output(ctx, runner.Cmd{Name: "getent", Args: []string{"passwd", user}})
	if err != nil {
		return "", fmt.Errorf("fish: read login shell: %w", err)
	}
	fields := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(fields) < 7 || fields[6] == "" {
		return "", &fishParseError{What: "passwd shell field", Output: string(out)}
	}
	return fields[6], nil
}

// fishChshCmd returns the interactive command that makes path the login shell
// of user. On Linux chsh asks for the user's password through PAM, which fails
// for users without one (cloud VMs), so it runs under sudo.
func fishChshCmd(p platform.Platform, path, user string) runner.Cmd {
	if p.OS == platform.Linux {
		return runner.Cmd{Name: "sudo", Args: []string{"chsh", "-s", path, user}, Interactive: true}
	}
	return runner.Cmd{Name: "chsh", Args: []string{"-s", path}, Interactive: true}
}

func fishHasFisher(ctx context.Context, env module.Env) (bool, error) {
	err := env.Run.Run(ctx, runner.Cmd{Name: "fish", Args: []string{"-c", "type -q fisher"}})
	var cmdErr *runner.CommandError
	if errors.As(err, &cmdErr) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("fish: check fisher: %w", err)
	}
	return true, nil
}

func (fishModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	path, err := fishPath(ctx, env)
	if err != nil {
		var cmdErr *runner.CommandError
		if errors.As(err, &cmdErr) {
			return module.StatusMissing, nil
		}
		return module.StatusMissing, err
	}
	listed, err := fishInShells(env, path)
	if err != nil || !listed {
		return module.StatusMissing, err
	}
	user, err := fishUser(ctx, env)
	if err != nil {
		return module.StatusMissing, err
	}
	shell, err := fishLoginShell(ctx, env, user)
	if err != nil || shell != path {
		return module.StatusMissing, err
	}
	hasFisher, err := fishHasFisher(ctx, env)
	if err != nil || !hasFisher {
		return module.StatusMissing, err
	}
	return module.StatusInstalled, nil
}

func (fishModule) Apply(ctx context.Context, env module.Env) error {
	path, err := fishPath(ctx, env)
	if err != nil {
		return err
	}
	listed, err := fishInShells(env, path)
	if err != nil {
		return err
	}
	if !listed {
		script := "echo '" + strings.ReplaceAll(path, "'", `'\''`) + "' >> " + fishShellsFile
		if err := env.Run.Run(ctx, runner.Cmd{Name: "sudo", Args: []string{"sh", "-c", script}, Interactive: true}); err != nil {
			return fmt.Errorf("fish: add to %s: %w", fishShellsFile, err)
		}
	}
	user, err := fishUser(ctx, env)
	if err != nil {
		return err
	}
	shell, err := fishLoginShell(ctx, env, user)
	if err != nil {
		return err
	}
	if shell != path {
		ok, err := env.Confirm("Make fish your login shell?", true)
		if err != nil {
			return fmt.Errorf("fish: confirm chsh: %w", err)
		}
		if ok {
			if err := env.Run.Run(ctx, fishChshCmd(env.Platform, path, user)); err != nil {
				return fmt.Errorf("fish: chsh: %w", err)
			}
		}
	}
	if err := env.Run.Run(ctx, runner.Cmd{Name: "fish", Args: []string{"-c", "fisher update"}}); err != nil {
		return fmt.Errorf("fish: fisher update: %w", err)
	}
	return nil
}
