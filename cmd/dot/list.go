package main

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/doraeminemon/dotfiles/internal/module"
)

// maxStatusLen caps the error text in the STATUS column; install prints
// errors in full.
const maxStatusLen = 100

// oneLine collapses the whitespace and line breaks of s to single spaces and
// cuts it to at most n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// runList prints every module with its platform support and Check status.
// With doctor set, it returns 1 when a supported module is not installed.
func runList(ctx context.Context, args []string, d deps, doctor bool) int {
	name := "list"
	if doctor {
		name = "doctor"
	}
	fl, repo := newFlagSet(name, d)
	if code, ok := parseFlags(fl, args); !ok {
		return code
	}
	if fl.NArg() > 0 {
		_, _ = fmt.Fprintf(d.stderr, "dot %s: unexpected argument %q\n", name, fl.Arg(0))
		return 2
	}
	s, err := newSetup(d, *repo, false, nil)
	if err != nil {
		return report(d.stderr, nil, err)
	}
	env := quietCheckEnv(s.env, d)

	tw := tabwriter.NewWriter(d.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSUPPORTED\tSTATUS\tSUMMARY")
	healthy := true
	for _, m := range s.reg.All() {
		supported, status := "no", "unsupported"
		if m.Supports(d.platform) {
			supported = "yes"
			st, err := m.Check(ctx, env)
			switch {
			case err != nil:
				status = "error: " + oneLine(err.Error(), maxStatusLen)
			default:
				status = st.String()
			}
			if err != nil || st != module.StatusInstalled {
				healthy = false
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.ID(), supported, status, m.Summary())
	}
	if err := tw.Flush(); err != nil {
		_, _ = fmt.Fprintln(d.stderr, "dot:", err)
		return 1
	}
	if doctor && !healthy {
		_, _ = fmt.Fprintln(d.stderr, "dot doctor: some modules are not installed; run dot install")
		return 1
	}
	return 0
}
