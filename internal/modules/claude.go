package modules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	claudeID = module.ID("claude")
	// claudeInstallScript is the official native installer from the Claude Code docs.
	claudeInstallScript = "curl -fsSL https://claude.ai/install.sh | bash"
	claudeCask          = "claude-code"
)

// ClaudeCaskMissingError reports that Claude Code is absent on darwin, where
// the brew module installs it from the cask list.
type ClaudeCaskMissingError struct {
	Cask string
}

func (e *ClaudeCaskMissingError) Error() string {
	return fmt.Sprintf("claude is not installed: the brew module should have installed cask %q; run the brew module first", e.Cask)
}

type claudeModule struct{}

// NewClaude returns the Claude Code installer module.
func NewClaude() module.Module { return claudeModule{} }

func (claudeModule) ID() module.ID { return claudeID }

func (claudeModule) Summary() string { return "Install the Claude Code CLI" }

func (claudeModule) DependsOn() []module.ID { return []module.ID{"brew"} }

func (claudeModule) Supports(p platform.Platform) bool {
	return p.OS == platform.Darwin || p.OS == platform.Linux
}

func (m claudeModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	if !m.Supports(env.Platform) {
		return module.StatusUnsupported, nil
	}
	if _, err := env.Run.Output(ctx, runner.Cmd{Name: "claude", Args: []string{"--version"}}); err == nil {
		return module.StatusInstalled, nil
	}
	if env.Platform.OS == platform.Linux {
		// PATH may not include ~/.local/bin right after the native install.
		_, err := env.FS.Stat(filepath.Join(env.Home, ".local", "bin", "claude"))
		switch {
		case err == nil:
			return module.StatusInstalled, nil
		case !errors.Is(err, fs.ErrNotExist):
			return module.StatusMissing, fmt.Errorf("stat claude binary: %w", err)
		}
	}
	return module.StatusMissing, nil
}

func (m claudeModule) Apply(ctx context.Context, env module.Env) error {
	st, err := m.Check(ctx, env)
	if err != nil {
		return err
	}
	switch st {
	case module.StatusInstalled, module.StatusUnsupported:
		return nil
	case module.StatusMissing:
	}
	if env.Platform.OS == platform.Darwin {
		return &ClaudeCaskMissingError{Cask: claudeCask}
	}
	if err := env.Run.Run(ctx, runner.Cmd{
		Name:        "bash",
		Args:        []string{"-c", claudeInstallScript},
		Interactive: true,
	}); err != nil {
		return fmt.Errorf("install claude: %w", err)
	}
	return nil
}
