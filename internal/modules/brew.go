package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/pkglist"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	brewID = module.ID("brew")

	// brewInstallerScript pipes the official Homebrew installer into bash.
	brewInstallerScript = "curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | /bin/bash"

	brewFormulaList = "brew.txt"
	brewCaskList    = "brew-cask-darwin.txt"
)

type brewModule struct {
	prependPath func(dirs ...string) error
}

// NewBrew returns the brew module. It installs Homebrew when it is missing,
// then the missing formulae from packages/brew.txt and, on darwin, the missing
// casks from packages/brew-cask-darwin.txt.
//
// prependPath puts directories in front of the PATH of the running process,
// so the modules after brew find the tools it installed. Apply calls it with
// Homebrew's bin and sbin directories.
func NewBrew(prependPath func(dirs ...string) error) module.Module {
	return &brewModule{prependPath: prependPath}
}

func (m *brewModule) ID() module.ID { return brewID }

func (m *brewModule) Summary() string {
	return "Homebrew and the formulae (and macOS casks) in packages/"
}

func (m *brewModule) DependsOn() []module.ID { return []module.ID{"repo", "apt"} }

func (m *brewModule) Supports(p platform.Platform) bool {
	_, ok := brewPrefix(p)
	return ok
}

func (m *brewModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	prefix, ok := brewPrefix(env.Platform)
	if !ok {
		return module.StatusUnsupported, nil
	}
	lists, err := brewReadLists(env)
	if err != nil {
		return module.StatusMissing, err
	}
	present, err := brewPresent(env, prefix)
	if err != nil {
		return module.StatusMissing, err
	}
	if !present {
		return module.StatusMissing, nil
	}
	missing, err := brewMissing(ctx, env, prefix, lists)
	if err != nil {
		return module.StatusMissing, err
	}
	if missing.empty() {
		return module.StatusInstalled, nil
	}
	return module.StatusMissing, nil
}

func (m *brewModule) Apply(ctx context.Context, env module.Env) error {
	prefix, ok := brewPrefix(env.Platform)
	if !ok {
		return &module.UnsupportedError{ID: brewID, Platform: env.Platform}
	}
	lists, err := brewReadLists(env)
	if err != nil {
		return err
	}
	present, err := brewPresent(env, prefix)
	if err != nil {
		return err
	}
	if !present {
		installer := runner.Cmd{
			Name:        "/bin/bash",
			Args:        []string{"-c", brewInstallerScript},
			Env:         []string{"NONINTERACTIVE=1"},
			Interactive: true,
		}
		if err := env.Run.Run(ctx, installer); err != nil {
			return fmt.Errorf("brew: install Homebrew: %w", err)
		}
	}
	if err := m.prependPath(prefix+"/bin", prefix+"/sbin"); err != nil {
		return fmt.Errorf("brew: put Homebrew on PATH: %w", err)
	}

	missing, err := brewMissing(ctx, env, prefix, lists)
	if err != nil {
		return err
	}
	brew := brewBin(prefix)
	if len(missing.formulae) > 0 {
		c := runner.Cmd{Name: brew, Args: append([]string{"install"}, brewNames(missing.formulae)...)}
		if err := env.Run.Run(ctx, c); err != nil {
			return fmt.Errorf("brew: install formulae: %w", err)
		}
	}
	if len(missing.casks) > 0 {
		// Some casks ask for the sudo password, so the cask install gets the
		// terminal.
		c := runner.Cmd{
			Name:        brew,
			Args:        append([]string{"install", "--cask"}, brewNames(missing.casks)...),
			Interactive: true,
		}
		if err := env.Run.Run(ctx, c); err != nil {
			return fmt.Errorf("brew: install casks: %w", err)
		}
	}
	return nil
}

// brewPrefix returns the Homebrew prefix on p, and false when brew does not
// support p.
func brewPrefix(p platform.Platform) (string, bool) {
	switch p.OS {
	case platform.Darwin:
		switch p.Arch {
		case platform.ARM64:
			return "/opt/homebrew", true
		case platform.AMD64:
			return "/usr/local", true
		}
	case platform.Linux:
		switch p.Arch {
		case platform.AMD64, platform.ARM64:
			return "/home/linuxbrew/.linuxbrew", true
		}
	}
	return "", false
}

func brewBin(prefix string) string { return prefix + "/bin/brew" }

// brewPresent reports whether the brew binary exists under prefix.
func brewPresent(env module.Env, prefix string) (bool, error) {
	_, err := env.FS.Stat(brewBin(prefix))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("brew: stat %s: %w", brewBin(prefix), err)
	}
}

// brewLists holds the wanted packages. casks is empty off darwin.
type brewLists struct {
	formulae []pkglist.Package
	casks    []pkglist.Package
}

func (l brewLists) empty() bool { return len(l.formulae) == 0 && len(l.casks) == 0 }

func brewReadLists(env module.Env) (brewLists, error) {
	formulae, err := brewReadList(env, brewFormulaList)
	if err != nil {
		return brewLists{}, err
	}
	lists := brewLists{formulae: formulae}
	if env.Platform.OS == platform.Darwin {
		casks, err := brewReadList(env, brewCaskList)
		if err != nil {
			return brewLists{}, err
		}
		lists.casks = casks
	}
	return lists, nil
}

func brewReadList(env module.Env, name string) ([]pkglist.Package, error) {
	file := filepath.Join(env.RepoDir, "packages", name)
	data, err := env.FS.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("brew: read %s: %w", file, err)
	}
	pkgs, err := pkglist.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("brew: parse %s: %w", file, err)
	}
	return pkgs, nil
}

// brewMissing returns the wanted packages that brew does not list as
// installed. It lists casks only when some are wanted.
func brewMissing(ctx context.Context, env module.Env, prefix string, want brewLists) (brewLists, error) {
	formulae, err := brewMissingKind(ctx, env, prefix, "--formula", want.formulae)
	if err != nil {
		return brewLists{}, err
	}
	var casks []pkglist.Package
	if len(want.casks) > 0 {
		casks, err = brewMissingKind(ctx, env, prefix, "--cask", want.casks)
		if err != nil {
			return brewLists{}, err
		}
	}
	return brewLists{formulae: formulae, casks: casks}, nil
}

func brewMissingKind(ctx context.Context, env module.Env, prefix, kind string, want []pkglist.Package) ([]pkglist.Package, error) {
	out, err := env.Run.Output(ctx, runner.Cmd{Name: brewBin(prefix), Args: []string{"list", kind, "-1"}})
	if err != nil {
		return nil, fmt.Errorf("brew: list %s: %w", kind, err)
	}
	installed := make(map[string]struct{})
	for line := range strings.Lines(string(out)) {
		if name := strings.TrimSpace(line); name != "" {
			installed[name] = struct{}{}
		}
	}
	var missing []pkglist.Package
	for _, p := range want {
		// brew list prints a tap-qualified package (owner/tap/name) by its
		// last path segment.
		if _, ok := installed[path.Base(string(p))]; !ok {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

func brewNames(pkgs []pkglist.Package) []string {
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = string(p)
	}
	return names
}
