// Command dot sets up a new macOS or Linux machine from this dotfiles repo.
//
// main is the program edge: it builds the real dependencies (platform, runner,
// prompter, file system, modules) and hands them to run, which tests drive with
// fakes.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/modules"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/project"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `dot %s

Usage:
  dot install [--only a,b] [--dry-run] [--yes] [--repo DIR]
  dot list [--repo DIR]
  dot doctor [--repo DIR]
  dot skills apply [--force] [--repo DIR] <profile> [dir]
  dot agents apply [--force] [--repo DIR] <template> [dir]
  dot version
  dot help
`

// deps is everything run needs from the outside world. main fills it with
// real implementations; tests fill it with fakes.
type deps struct {
	stdout, stderr io.Writer
	// stdinTTY reports whether stdin is a terminal, which allows the module
	// multiselect.
	stdinTTY bool
	platform platform.Platform
	// run executes commands for real, with their output shown.
	run runner.Runner
	// query executes the read-only Output queries of dry runs and Check,
	// without echoing their stderr (see dryRunner).
	query runner.Runner
	ask   prompt.Prompter
	// fs is the real file system. Dry runs wrap it (see dryFS).
	fs   project.FS
	cwd  string
	home string
	// modules are registered in this order.
	modules []module.Module
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := mainCode(ctx)
	stop()
	os.Exit(code)
}

func mainCode(ctx context.Context) int {
	if err := prependBrewPaths(os.Stat, prependPath); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dot:", err)
		return 1
	}
	p, err := platform.Current()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dot:", err)
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dot:", err)
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dot:", err)
		return 1
	}
	d := deps{
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		stdinTTY: term.IsTerminal(int(os.Stdin.Fd())),
		platform: p,
		run:      &runner.ExecRunner{Log: os.Stdout},
		query:    &runner.ExecRunner{},
		ask:      prompt.NewHuhPrompter(),
		fs:       project.OSFS{},
		cwd:      cwd,
		home:     home,
		modules:  allModules(),
	}
	return run(ctx, os.Args[1:], d)
}

// allModules returns every installer module in registration order.
func allModules() []module.Module {
	return []module.Module{
		modules.NewRepo(),
		modules.NewApt(),
		modules.NewBrew(prependPath),
		modules.NewChezmoi(time.Now),
		modules.NewSecrets(os.LookupEnv),
		modules.NewFish(),
		modules.NewMise(),
		modules.NewPython(),
		modules.NewClaude(),
		modules.NewSSH(),
		modules.NewSkills(),
	}
}

// brewPrefixes are the Homebrew install prefixes, in the order prependBrewPaths
// checks them.
var brewPrefixes = []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}

// prependBrewPaths puts the bin and sbin of every installed Homebrew on PATH, so
// a new dot process finds brew-installed tools before the shell restarts.
// Prefixes are visited in reverse, so the first listed one ends up first.
func prependBrewPaths(stat func(string) (os.FileInfo, error), prepend func(...string) error) error {
	for _, prefix := range slices.Backward(brewPrefixes) {
		if _, err := stat(prefix + "/bin/brew"); err != nil {
			continue
		}
		if err := prepend(prefix+"/bin", prefix+"/sbin"); err != nil {
			return fmt.Errorf("prepend %s to PATH: %w", prefix, err)
		}
	}
	return nil
}

// prependPath puts dirs in front of the PATH of this process, so later modules
// find the tools brew installed. It drops existing copies of dirs from PATH.
// It changes only this process, so dry runs call it too: later Check queries
// then find a Homebrew that exists but is not yet on PATH.
func prependPath(dirs ...string) error {
	old := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	next := slices.Clone(dirs)
	for _, p := range old {
		if p != "" && !slices.Contains(next, p) {
			next = append(next, p)
		}
	}
	return os.Setenv("PATH", strings.Join(next, string(os.PathListSeparator)))
}

// run executes one dot command and returns the process exit code.
func run(ctx context.Context, args []string, d deps) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(d.stderr, usage, version)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprintf(d.stdout, usage, version)
		return 0
	case "version", "--version":
		_, _ = fmt.Fprintln(d.stdout, version)
		return 0
	case "install":
		return runInstall(ctx, args[1:], d)
	case "list":
		return runList(ctx, args[1:], d, false)
	case "doctor":
		return runList(ctx, args[1:], d, true)
	case "skills", "agents":
		return runProject(ctx, args[0], args[1:], d)
	}
	_, _ = fmt.Fprintf(d.stderr, "dot: unknown command %q\n\n", args[0])
	_, _ = fmt.Fprintf(d.stderr, usage, version)
	return 2
}
