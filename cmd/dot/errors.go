package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/project"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// stderrTailLines is how many lines of a failed command's stderr are shown.
const stderrTailLines = 20

// report prints err to w with a hint for the typed errors, and returns the exit
// code for it. reg lists the valid module IDs; it may be nil.
func report(w io.Writer, reg *module.Registry, err error) int {
	var (
		unknownMod  *module.UnknownModuleError
		unsupported *module.UnsupportedError
		cycle       *module.CycleError
		cmdErr      *runner.CommandError
		notFound    *runner.NotFoundError
		unknownProf *project.UnknownProfileError
		unknownTmpl *project.UnknownTemplateError
		exists      *project.ExistsError
	)
	switch {
	case errors.Is(err, prompt.ErrAborted):
		_, _ = fmt.Fprintln(w, "dot: aborted")
		return 130
	case errors.As(err, &unknownMod):
		_, _ = fmt.Fprintf(w, "dot: %v\n", err)
		if reg != nil {
			_, _ = fmt.Fprintf(w, "valid modules: %s\n", strings.Join(moduleIDs(reg), ", "))
		}
		return 2
	case errors.As(err, &unsupported):
		_, _ = fmt.Fprintf(w, "dot: %v\n", err)
		return 2
	case errors.As(err, &cycle):
		_, _ = fmt.Fprintf(w, "dot: %v\n", err)
		return 1
	case errors.As(err, &notFound):
		_, _ = fmt.Fprintf(w, "dot: %v\ninstall %s first\n", err, notFound.Name)
		return 1
	case errors.As(err, &cmdErr):
		// cmdErr.Error() embeds all of stderr, so print the parts instead. The
		// step header above names the module.
		_, _ = fmt.Fprintf(w, "dot: command failed: %s (exit status %d)\n", cmdErr.Cmd, cmdErr.ExitCode)
		if tail := lastLines(cmdErr.Stderr, stderrTailLines); tail != "" {
			_, _ = fmt.Fprintf(w, "stderr:\n%s\n", tail)
		}
		return 1
	case errors.As(err, &unknownProf):
		_, _ = fmt.Fprintf(w, "dot: unknown skills profile %q\navailable: %s\n", unknownProf.Name, strings.Join(unknownProf.Available, ", "))
		return 2
	case errors.As(err, &unknownTmpl):
		_, _ = fmt.Fprintf(w, "dot: unknown agents template %q\navailable: %s\n", unknownTmpl.Name, strings.Join(unknownTmpl.Available, ", "))
		return 2
	case errors.As(err, &exists):
		_, _ = fmt.Fprintf(w, "dot: %v; use --force to overwrite\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "dot: %v\n", err)
	return 1
}

func moduleIDs(reg *module.Registry) []string {
	all := reg.All()
	ids := make([]string, len(all))
	for i, m := range all {
		ids[i] = string(m.ID())
	}
	return ids
}

// lastLines returns the last n non-empty-trailing lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
