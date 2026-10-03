package main

import (
	"context"
	"fmt"

	"github.com/doraeminemon/dotfiles/internal/project"
)

// runProject runs `dot skills apply` or `dot agents apply`.
func runProject(ctx context.Context, kind string, args []string, d deps) int {
	if len(args) == 0 || args[0] != "apply" {
		_, _ = fmt.Fprintf(d.stderr, "usage: dot %s apply [--force] [--repo DIR] <name> [dir]\n", kind)
		return 2
	}
	fl, repo := newFlagSet(kind+" apply", d)
	force := fl.Bool("force", false, "overwrite existing files")
	if code, ok := parseFlags(fl, args[1:]); !ok {
		return code
	}
	if fl.NArg() < 1 || fl.NArg() > 2 {
		_, _ = fmt.Fprintf(d.stderr, "usage: dot %s apply [--force] [--repo DIR] <name> [dir]\n", kind)
		return 2
	}
	repoDir := *repo
	if repoDir == "" {
		repoDir = findRepoDir(d.fs, d.cwd, d.home)
	}
	target := d.cwd
	if fl.NArg() == 2 {
		target = fl.Arg(1)
	}

	var err error
	switch kind {
	case "skills":
		var p project.ProfileName
		if p, err = project.ParseProfile(d.fs, repoDir, fl.Arg(0)); err == nil {
			err = project.ApplySkills(ctx, d.run, d.fs, repoDir, p, target, *force)
		}
	default:
		var t project.TemplateName
		if t, err = project.ParseTemplate(d.fs, repoDir, fl.Arg(0)); err == nil {
			err = project.ApplyAgents(d.fs, repoDir, t, target, *force)
		}
	}
	if err != nil {
		return report(d.stderr, nil, err)
	}
	_, _ = fmt.Fprintf(d.stdout, "Applied %s %s to %s\n", kind, fl.Arg(0), target)
	return 0
}
