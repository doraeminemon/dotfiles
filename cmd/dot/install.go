package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/prompt"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

// newFlagSet returns a FlagSet for subcommand name that reports to stderr and
// has the common --repo flag.
func newFlagSet(name string, d deps) (*flag.FlagSet, *string) {
	fl := flag.NewFlagSet("dot "+name, flag.ContinueOnError)
	fl.SetOutput(d.stderr)
	repo := fl.String("repo", "", "dotfiles checkout `dir` (default: the enclosing checkout, else ~/Projects/dotfiles)")
	return fl, repo
}

// parseFlags parses args into fl. It returns an exit code and false when the
// command must stop: 0 for -h, 2 for a bad flag.
func parseFlags(fl *flag.FlagSet, args []string) (int, bool) {
	err := fl.Parse(args)
	switch {
	case err == nil:
		return 0, true
	case errors.Is(err, flag.ErrHelp):
		return 0, false
	default:
		return 2, false
	}
}

// setup is the registry and Env that a module command runs with.
type setup struct {
	reg *module.Registry
	env module.Env
}

// newSetup builds the registry and an Env on the real runner and file system,
// or on the dry wrappers that print to out when dry is set.
func newSetup(d deps, repoFlag string, dry bool, out io.Writer) (setup, error) {
	reg, err := module.NewRegistry(d.modules...)
	if err != nil {
		return setup{}, err
	}
	repoDir := repoFlag
	if repoDir == "" {
		repoDir = findRepoDir(d.fs, d.cwd, d.home)
	}
	env := module.Env{
		Platform: d.platform,
		Run:      d.run,
		Ask:      d.ask,
		FS:       d.fs,
		RepoDir:  repoDir,
		Home:     d.home,
	}
	if dry {
		env.Run = &dryRunner{inner: d.query, out: out}
		env.FS = dryFS{inner: d.fs, out: out}
	}
	return setup{reg: reg, env: env}, nil
}

func runInstall(ctx context.Context, args []string, d deps) int {
	fl, repo := newFlagSet("install", d)
	only := fl.String("only", "", "comma-separated module `ids` to install, with their dependencies")
	dry := fl.Bool("dry-run", false, "print what would change and change nothing")
	yes := fl.Bool("yes", false, "answer yes to every confirmation and skip the module picker")
	if code, ok := parseFlags(fl, args); !ok {
		return code
	}
	if fl.NArg() > 0 {
		_, _ = fmt.Fprintf(d.stderr, "dot install: unexpected argument %q\n", fl.Arg(0))
		return 2
	}

	s, err := newSetup(d, *repo, *dry, d.stdout)
	if err != nil {
		return report(d.stderr, nil, err)
	}
	s.env.Yes = *yes

	selected, err := selectModules(ctx, s, d, *only, *yes)
	if err != nil {
		return report(d.stderr, s.reg, err)
	}
	if selected != nil && len(selected) == 0 {
		_, _ = fmt.Fprintln(d.stdout, "No modules selected.")
		return 0
	}
	plan, err := s.reg.Plan(selected, d.platform)
	if err != nil {
		return report(d.stderr, s.reg, err)
	}

	if *dry {
		_, _ = lipgloss.Fprintln(d.stdout, dimStyle.Render("Dry run: commands marked + are not run, ? are read-only queries."))
	}
	ids := make([]string, len(plan))
	for i, m := range plan {
		ids[i] = string(m.ID())
	}
	_, _ = fmt.Fprintf(d.stdout, "Plan: %s\n", strings.Join(ids, " → "))
	var failed []string
	for _, m := range plan {
		_, _ = lipgloss.Fprintln(d.stdout, headerStyle.Render(fmt.Sprintf("==> %s — %s", m.ID(), m.Summary())))
		err := runStep(ctx, m, s.env, d.stdout)
		if err == nil {
			continue
		}
		if !*dry || errors.Is(err, prompt.ErrAborted) {
			return report(d.stderr, s.reg, fmt.Errorf("%s: %w", m.ID(), err))
		}
		failed = append(failed, string(m.ID()))
		reportStep(d.stderr, s.reg, m.ID(), err)
	}
	if !*dry {
		return 0
	}
	if len(failed) == 0 {
		_, _ = fmt.Fprintf(d.stdout, "Dry run finished: all %d steps OK\n", len(plan))
		return 0
	}
	_, _ = fmt.Fprintf(d.stdout, "Dry run finished: %d of %d steps failed: %s\n", len(failed), len(plan), strings.Join(failed, ", "))
	return 1
}

// runStep checks m and applies it when it is missing. Check errors are
// prefixed with "check: ".
func runStep(ctx context.Context, m module.Module, env module.Env, out io.Writer) error {
	st, err := m.Check(ctx, env)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if st != module.StatusMissing {
		_, _ = lipgloss.Fprintln(out, dimStyle.Render("    already "+st.String()))
		return nil
	}
	return m.Apply(ctx, env)
}

// reportStep prints a failed dry-run step as "! <id>: <error>", with the error
// formatted as report formats it.
func reportStep(w io.Writer, reg *module.Registry, id module.ID, err error) {
	var b strings.Builder
	report(&b, reg, err)
	msg := strings.TrimSuffix(strings.TrimPrefix(b.String(), "dot: "), "\n")
	_, _ = fmt.Fprintf(w, "! %s: %s\n", id, msg)
}

// selectModules returns the modules the user asked for. A nil result means
// every module that supports the platform; an empty non-nil result means the
// user picked none.
func selectModules(ctx context.Context, s setup, d deps, only string, yes bool) ([]module.ID, error) {
	if only != "" {
		var ids []module.ID
		for name := range strings.SplitSeq(only, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id, err := s.reg.ParseID(name)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	}
	if !d.stdinTTY || yes {
		return nil, nil
	}

	// The picker shows each module's status. Check changes nothing, and the
	// quiet dry wrappers make sure of it.
	checkEnv := quietCheckEnv(s.env, d)
	var opts []prompt.Option
	for _, m := range s.reg.All() {
		if !m.Supports(d.platform) {
			continue
		}
		opts = append(opts, prompt.Option{
			Label:    fmt.Sprintf("%s — %s (%s)", m.ID(), m.Summary(), statusText(ctx, m, checkEnv)),
			Value:    string(m.ID()),
			Selected: true,
		})
	}
	picked, err := d.ask.MultiSelect("Modules to install", opts)
	if err != nil {
		return nil, err
	}
	ids := make([]module.ID, 0, len(picked))
	for _, name := range picked {
		id, err := s.reg.ParseID(name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// quietCheckEnv returns env on dry wrappers that print nothing, for running
// Check outside an install.
func quietCheckEnv(env module.Env, d deps) module.Env {
	env.Run = &dryRunner{inner: d.query, out: io.Discard}
	env.FS = dryFS{inner: d.fs, out: io.Discard}
	return env
}

// statusText runs m.Check and returns its status, or "error: <msg>".
func statusText(ctx context.Context, m module.Module, env module.Env) string {
	st, err := m.Check(ctx, env)
	if err != nil {
		return "error: " + err.Error()
	}
	return st.String()
}
