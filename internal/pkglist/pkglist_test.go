package pkglist

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Package
	}{
		{"empty", "", nil},
		{"comment only", "# a\n   # b\n\n", nil},
		{"inline comments", "git # vcs\n  fish  \n#x\n", []Package{"git", "fish"}},
		{"tap and version", "anomalyco/tap/opencode\nnode@22\ng++\n", []Package{"anomalyco/tap/opencode", "node@22", "g++"}},
		{"crlf", "git\r\nfish\r\n", []Package{"git", "fish"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseInvalidLine(t *testing.T) {
	tests := []struct {
		name, input, text string
	}{
		{"space", "git\nfoo bar\n", "foo bar"},
		{"symbol", "git\nfoo$\n", "foo$"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.input))
			var ile *InvalidLineError
			if !errors.As(err, &ile) {
				t.Fatalf("got %v, want InvalidLineError", err)
			}
			if ile.Line != 2 || ile.Text != tc.text {
				t.Errorf("got %+v", ile)
			}
		})
	}
}

func TestParseDuplicate(t *testing.T) {
	_, err := Parse(strings.NewReader("git\n# c\nfish\ngit # again\n"))
	var de *DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("got %v, want DuplicateError", err)
	}
	if de.Package != "git" || de.FirstLine != 1 || de.Line != 4 {
		t.Errorf("got %+v", de)
	}
}

func TestParseFile(t *testing.T) {
	fsys := fstest.MapFS{
		"packages/brew.txt": {Data: []byte("git\nfish\n")},
		"packages/bad.txt":  {Data: []byte("a b\n")},
	}
	got, err := ParseFile(fsys, "packages/brew.txt")
	if err != nil || !slices.Equal(got, []Package{"git", "fish"}) {
		t.Fatalf("got %v, %v", got, err)
	}

	_, err = ParseFile(fsys, "packages/missing.txt")
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "packages/missing.txt") {
		t.Fatalf("got %v", err)
	}

	_, err = ParseFile(fsys, "packages/bad.txt")
	var ile *InvalidLineError
	if !errors.As(err, &ile) || !strings.Contains(err.Error(), "packages/bad.txt") {
		t.Fatalf("got %v", err)
	}
}
