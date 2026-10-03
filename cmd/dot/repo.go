package main

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
)

const modulePath = "github.com/doraeminemon/dotfiles"

// findRepoDir returns the nearest directory at or above cwd whose go.mod
// declares modulePath, so `go run ./cmd/dot` uses its own checkout. Otherwise
// it returns <home>/Projects/dotfiles, where the repo module clones the repo.
func findRepoDir(fsys module.WriteFS, cwd, home string) string {
	for dir := cwd; ; {
		if data, err := fsys.ReadFile(filepath.Join(dir, "go.mod")); err == nil && goModPath(data) == modulePath {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(home, "Projects", "dotfiles")
}

// goModPath returns the path of the first module directive in a go.mod file,
// or "" when there is none.
func goModPath(data []byte) string {
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		rest, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module")
		if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = rest[:i]
		}
		return strings.Trim(strings.TrimSpace(rest), `"`)
	}
	return ""
}
