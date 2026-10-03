package modules

import (
	"context"
	"fmt"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const miseID module.ID = "mise"

// miseModule installs the tools that the global mise config lists.
type miseModule struct{}

// NewMise returns the module that runs `mise install` for the global config.
func NewMise() module.Module { return miseModule{} }

func (miseModule) ID() module.ID { return miseID }

func (miseModule) Summary() string {
	return "Install the tools in the global mise config"
}

// DependsOn: brew provides mise, chezmoi writes ~/.config/mise/config.toml.
func (miseModule) DependsOn() []module.ID {
	return []module.ID{"brew", "chezmoi"}
}

func (miseModule) Supports(platform.Platform) bool { return true }

// miseCmd runs mise from the home directory, so a project-local config
// cannot change the result.
func miseCmd(env module.Env, args ...string) runner.Cmd {
	return runner.Cmd{Name: "mise", Args: args, Dir: env.Home}
}

// Check is installed when `mise ls --missing --global` prints nothing.
func (miseModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	out, err := env.Run.Output(ctx, miseCmd(env, "ls", "--missing", "--global"))
	if err != nil {
		return module.StatusMissing, fmt.Errorf("mise: list missing tools: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return module.StatusInstalled, nil
	}
	return module.StatusMissing, nil
}

// Apply installs every tool in the global config. Installed tools are skipped
// by mise, so repeat runs change nothing.
func (miseModule) Apply(ctx context.Context, env module.Env) error {
	if err := env.Run.Run(ctx, miseCmd(env, "install", "--yes")); err != nil {
		return fmt.Errorf("mise: install tools: %w", err)
	}
	return nil
}
