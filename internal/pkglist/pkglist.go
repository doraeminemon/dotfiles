// Package pkglist parses package list files: one package name per line,
// with '#' comments and blank lines ignored.
package pkglist

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// Package is a validated package name. Only Parse and ParseFile create values.
type Package string

// InvalidLineError reports a line whose name contains a disallowed character.
type InvalidLineError struct {
	Line int
	Text string
}

func (e *InvalidLineError) Error() string {
	return fmt.Sprintf("line %d: invalid package name %q", e.Line, e.Text)
}

// DuplicateError reports a package listed more than once.
type DuplicateError struct {
	Package   Package
	FirstLine int
	Line      int
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("line %d: duplicate package %q (first on line %d)", e.Line, e.Package, e.FirstLine)
}

// Parse reads a package list. A '#' starts a comment anywhere on a line.
func Parse(r io.Reader) ([]Package, error) {
	var pkgs []Package
	seen := map[Package]int{}
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text, _, _ := strings.Cut(sc.Text(), "#")
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if !validName(text) {
			return nil, &InvalidLineError{Line: line, Text: text}
		}
		p := Package(text)
		if first, ok := seen[p]; ok {
			return nil, &DuplicateError{Package: p, FirstLine: first, Line: line}
		}
		seen[p] = line
		pkgs = append(pkgs, p)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read package list: %w", err)
	}
	return pkgs, nil
}

// ParseFile parses the named file in fsys. Errors are wrapped with the name.
func ParseFile(fsys fs.FS, name string) ([]Package, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	pkgs, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return pkgs, nil
}

func validName(s string) bool {
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("@._+-/", c):
		default:
			return false
		}
	}
	return true
}
