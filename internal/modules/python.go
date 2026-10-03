package modules

import (
	"bufio"
	"bytes"
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

const pythonID module.ID = "python"

// pythonTool is a uv tool that the python module installs.
type pythonTool string

// pythonTools are installed with `uv tool install`, in this order.
var pythonTools = [...]pythonTool{"ruff", "ty"}

// pythonInstallerURLPrefix is the conda-forge release download base. The
// asset name ends with the `uname -m` spelling of the architecture.
const pythonInstallerURLPrefix = "https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-"

type pythonModule struct{}

// NewPython returns the module that sets up conda (miniforge) and the uv
// Python tools ruff and ty.
//
// On darwin the miniforge cask comes from the brew module. On linux the
// module downloads the Miniforge installer into ~/miniforge3. On both, it
// turns off auto_activate_base and installs the missing uv tools. It does not
// run `conda init fish`, because that writes absolute paths into config.fish.
// The tracked config.fish loads the conda hook itself.
func NewPython() module.Module { return pythonModule{} }

func (pythonModule) ID() module.ID { return pythonID }

func (pythonModule) Summary() string {
	return "conda (miniforge) and the uv tools ruff and ty"
}

func (pythonModule) DependsOn() []module.ID { return []module.ID{"brew"} }

func (pythonModule) Supports(p platform.Platform) bool {
	_, err := pythonCondaBase(p, "")
	return err == nil
}

func (m pythonModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	base, err := pythonCondaBase(env.Platform, env.Home)
	if err != nil {
		return module.StatusUnsupported, nil
	}
	ok, err := pythonCondaExists(env.FS, base)
	if err != nil || !ok {
		return module.StatusMissing, err
	}
	installed, err := pythonInstalledTools(ctx, env.Run)
	if err != nil {
		return module.StatusMissing, err
	}
	for _, t := range pythonTools {
		if !installed[t] {
			return module.StatusMissing, nil
		}
	}
	return module.StatusInstalled, nil
}

func (m pythonModule) Apply(ctx context.Context, env module.Env) error {
	base, err := pythonCondaBase(env.Platform, env.Home)
	if err != nil {
		return err
	}
	if env.Platform.OS == platform.Linux {
		if err := pythonInstallLinux(ctx, env, base); err != nil {
			return err
		}
	}
	conda := pythonConda(base)
	args := []string{"config", "--set", "auto_activate_base", "false"}
	if err := env.Run.Run(ctx, runner.Cmd{Name: conda, Args: args}); err != nil {
		return fmt.Errorf("python: conda %s: %w", strings.Join(args, " "), err)
	}
	installed, err := pythonInstalledTools(ctx, env.Run)
	if err != nil {
		return err
	}
	for _, t := range pythonTools {
		if installed[t] {
			continue
		}
		c := runner.Cmd{Name: "uv", Args: []string{"tool", "install", string(t)}}
		if err := env.Run.Run(ctx, c); err != nil {
			return fmt.Errorf("python: uv tool install %s: %w", t, err)
		}
	}
	return nil
}

// pythonCondaBase returns the miniforge prefix on p. On darwin it is the
// Caskroom of the miniforge cask (`brew info --cask miniforge` lists
// <Caskroom>/miniforge/base/condabin/conda). On linux it is ~/miniforge3.
// It returns *module.UnsupportedError for a Platform outside the enums.
func pythonCondaBase(p platform.Platform, home string) (string, error) {
	switch p.OS {
	case platform.Darwin:
		switch p.Arch {
		case platform.ARM64:
			return "/opt/homebrew/Caskroom/miniforge/base", nil
		case platform.AMD64:
			return "/usr/local/Caskroom/miniforge/base", nil
		}
	case platform.Linux:
		switch p.Arch {
		case platform.ARM64, platform.AMD64:
			return filepath.Join(home, "miniforge3"), nil
		}
	}
	return "", &module.UnsupportedError{ID: pythonID, Platform: p}
}

// pythonConda is the absolute path of the conda binary under base. The
// module never resolves conda through PATH, because conda init has not run
// yet on a new machine.
func pythonConda(base string) string { return filepath.Join(base, "bin", "conda") }

func pythonCondaExists(fsys module.WriteFS, base string) (bool, error) {
	_, err := fsys.Stat(pythonConda(base))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("python: stat conda: %w", err)
	}
}

// pythonInstallLinux downloads and runs the Miniforge installer into base,
// unless conda is already there.
func pythonInstallLinux(ctx context.Context, env module.Env, base string) error {
	ok, err := pythonCondaExists(env.FS, base)
	if err != nil || ok {
		return err
	}
	tmp := filepath.Join(env.Home, ".cache", "miniforge-installer.sh")
	if err := env.FS.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		return fmt.Errorf("python: create installer dir: %w", err)
	}
	url := pythonInstallerURLPrefix + env.Platform.UnameMachine() + ".sh"
	q := pythonShellQuote
	script := "curl -fsSL " + q(url) + " -o " + q(tmp) +
		" && bash " + q(tmp) + " -b -p " + q(base) +
		" && rm -f " + q(tmp)
	c := runner.Cmd{Name: "sh", Args: []string{"-c", script}, Interactive: true}
	if err := env.Run.Run(ctx, c); err != nil {
		return fmt.Errorf("python: install miniforge: %w", err)
	}
	return nil
}

// pythonInstalledTools parses `uv tool list`. Each installed tool starts a
// line as "<name> v<version>"; the lines under it list its executables with
// a leading "- ". A uv that is not on PATH has no tools.
func pythonInstalledTools(ctx context.Context, r runner.Runner) (map[pythonTool]bool, error) {
	out, err := r.Output(ctx, runner.Cmd{Name: "uv", Args: []string{"tool", "list"}})
	var nf *runner.NotFoundError
	if errors.As(err, &nf) {
		return map[pythonTool]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("python: uv tool list: %w", err)
	}
	found := make(map[pythonTool]bool, len(pythonTools))
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		for _, t := range pythonTools {
			if strings.HasPrefix(line, string(t)+" ") {
				found[t] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("python: read uv tool list: %w", err)
	}
	return found, nil
}

// pythonShellQuote single-quotes s for sh unless it holds only safe
// characters, so a home directory with spaces stays one word.
func pythonShellQuote(s string) string {
	safe := strings.IndexFunc(s, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		}
		return !strings.ContainsRune("-_./:=@%+,", r)
	}) < 0
	if s != "" && safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
