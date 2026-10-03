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
	"github.com/doraeminemon/dotfiles/internal/pkglist"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// aptDefaultList is the fallback for packages/apt.txt. On a fresh machine the
// repo is not cloned when apt runs. A test keeps it equal to the repo file.
const aptDefaultList = `# apt packages, installed on Debian/Ubuntu only (checked against Ubuntu 24.04).
# System prerequisites for Homebrew on Linux, plus fish as the login shell.
# All other tools come from brew.txt.
# One package per line. ` + "`#`" + ` starts a comment. Blank lines are ignored.

build-essential
curl
git
file
procps
unzip
ca-certificates
fish
`

const aptInstalledStatus = "install ok installed"

type aptModule struct{}

// NewApt returns the module that installs the apt package list on Debian-based Linux.
func NewApt() module.Module { return aptModule{} }

func (aptModule) ID() module.ID { return "apt" }

func (aptModule) Summary() string { return "Install system packages with apt" }

func (aptModule) DependsOn() []module.ID { return nil }

func (aptModule) Supports(p platform.Platform) bool {
	return p.OS == platform.Linux && p.PkgMgr == platform.Apt
}

// aptPackages reads packages/apt.txt from the repo, or the built-in list when
// the repo is not cloned yet.
func aptPackages(env module.Env) ([]pkglist.Package, error) {
	path := filepath.Join(env.RepoDir, "packages", "apt.txt")
	data, err := env.FS.ReadFile(path)
	switch {
	case err == nil:
	case errors.Is(err, fs.ErrNotExist):
		data = []byte(aptDefaultList)
	default:
		return nil, fmt.Errorf("apt: read %s: %w", path, err)
	}
	pkgs, err := pkglist.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("apt: parse package list: %w", err)
	}
	return pkgs, nil
}

// aptMissing returns the listed packages that dpkg does not report as installed.
func aptMissing(ctx context.Context, env module.Env) ([]pkglist.Package, error) {
	pkgs, err := aptPackages(env)
	if err != nil {
		return nil, err
	}
	var missing []pkglist.Package
	for _, pkg := range pkgs {
		out, err := env.Run.Output(ctx, runner.Cmd{
			Name: "dpkg-query",
			Args: []string{"-W", "-f=${Status}", string(pkg)},
		})
		var cmdErr *runner.CommandError
		switch {
		case err == nil:
			if !strings.Contains(string(out), aptInstalledStatus) {
				missing = append(missing, pkg)
			}
		case errors.As(err, &cmdErr):
			// dpkg-query exits non-zero for a package it does not know.
			missing = append(missing, pkg)
		default:
			return nil, fmt.Errorf("apt: check %s: %w", pkg, err)
		}
	}
	return missing, nil
}

func (aptModule) Check(ctx context.Context, env module.Env) (module.Status, error) {
	missing, err := aptMissing(ctx, env)
	if err != nil {
		return module.StatusMissing, err
	}
	if len(missing) > 0 {
		return module.StatusMissing, nil
	}
	return module.StatusInstalled, nil
}

func (aptModule) Apply(ctx context.Context, env module.Env) error {
	missing, err := aptMissing(ctx, env)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	update := runner.Cmd{Name: "sudo", Args: []string{"apt-get", "update"}, Interactive: true}
	if err := env.Run.Run(ctx, update); err != nil {
		return fmt.Errorf("apt: update: %w", err)
	}
	args := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y"}
	for _, pkg := range missing {
		args = append(args, string(pkg))
	}
	if err := env.Run.Run(ctx, runner.Cmd{Name: "sudo", Args: args, Interactive: true}); err != nil {
		return fmt.Errorf("apt: install: %w", err)
	}
	return nil
}
