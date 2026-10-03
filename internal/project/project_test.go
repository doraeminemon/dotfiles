package project

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// testFS is a module.MemFS that also lists directories. It records the
// children of each directory that a file was written into through put.
type testFS struct {
	*module.MemFS
	children map[string][]string
}

func newTestFS(t *testing.T, files map[string]string) *testFS {
	t.Helper()
	f := &testFS{MemFS: &module.MemFS{}, children: map[string][]string{}}
	for name, data := range files {
		dir := filepath.Dir(name)
		if err := f.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := f.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		f.children[dir] = append(f.children[dir], filepath.Base(name))
	}
	return f
}

func (f *testFS) ReadDir(name string) ([]fs.DirEntry, error) {
	kids, ok := f.children[filepath.Clean(name)]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	kids = slices.Sorted(slices.Values(kids))
	out := make([]fs.DirEntry, 0, len(kids))
	for _, k := range kids {
		info, err := f.Stat(filepath.Join(name, k))
		if err != nil {
			return nil, err
		}
		out = append(out, fs.FileInfoToDirEntry(info))
	}
	return out, nil
}

const repo = "/repo"

func repoFS(t *testing.T) *testFS {
	t.Helper()
	return newTestFS(t, map[string]string{
		"/repo/skills/profiles/rust.json": `{"version":1,"skills":{}}`,
		"/repo/skills/profiles/go.json":   "{\"version\": 1}\n",
		"/repo/skills/profiles/notes.txt": "not a profile",
		"/repo/agents/README.md":          "readme",
		"/repo/agents/CLAUDE.md":          "@AGENTS.md\n",
		"/repo/agents/rust.md":            "# rust\n",
		"/repo/agents/go.md":              "# go\r\nbody\x00\n",
		"/repo/agents/base.md":            "# base\n",
		"/work/keep.txt":                  "",
	})
}

func readString(t *testing.T, fsys FS, name string) string {
	t.Helper()
	b, err := fsys.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseProfile(t *testing.T) {
	fsys := repoFS(t)
	got, err := ParseProfile(fsys, repo, "go")
	if err != nil || got != "go" {
		t.Fatalf("ParseProfile(go) = %q, %v", got, err)
	}

	for _, name := range []string{"python", "notes", "../agents/go", ""} {
		_, err := ParseProfile(fsys, repo, name)
		var ue *UnknownProfileError
		if !errors.As(err, &ue) {
			t.Fatalf("ParseProfile(%q) err = %v, want *UnknownProfileError", name, err)
		}
		if ue.Name != name || !slices.Equal(ue.Available, []string{"go", "rust"}) {
			t.Errorf("ParseProfile(%q) err = %+v", name, ue)
		}
	}
}

func TestParseTemplate(t *testing.T) {
	fsys := repoFS(t)
	got, err := ParseTemplate(fsys, repo, "rust")
	if err != nil || got != "rust" {
		t.Fatalf("ParseTemplate(rust) = %q, %v", got, err)
	}

	for _, name := range []string{"python", "CLAUDE", "README"} {
		_, err := ParseTemplate(fsys, repo, name)
		var ue *UnknownTemplateError
		if !errors.As(err, &ue) {
			t.Fatalf("ParseTemplate(%q) err = %v, want *UnknownTemplateError", name, err)
		}
		if ue.Name != name || !slices.Equal(ue.Available, []string{"base", "go", "rust"}) {
			t.Errorf("ParseTemplate(%q) err = %+v", name, ue)
		}
	}
}

func TestParseMissingDir(t *testing.T) {
	fsys := newTestFS(t, nil)
	if _, err := ParseProfile(fsys, repo, "go"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ParseProfile err = %v, want fs.ErrNotExist", err)
	}
	if _, err := ParseTemplate(fsys, repo, "go"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ParseTemplate err = %v, want fs.ErrNotExist", err)
	}
}

// TestParseRealRepo checks the parse functions against this checkout's data.
func TestParseRealRepo(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := ParseProfile(OSFS{}, root, "go"); err != nil {
		t.Errorf("ParseProfile(go): %v", err)
	}
	_, err := ParseTemplate(OSFS{}, root, "CLAUDE")
	var ue *UnknownTemplateError
	if !errors.As(err, &ue) {
		t.Fatalf("ParseTemplate(CLAUDE) err = %v", err)
	}
	if !slices.Contains(ue.Available, "go") || slices.Contains(ue.Available, "README") || !slices.IsSorted(ue.Available) {
		t.Errorf("Available = %v", ue.Available)
	}
}

func TestApplySkills(t *testing.T) {
	fsys := repoFS(t)
	run := &runner.FakeRunner{}
	if err := ApplySkills(context.Background(), run, fsys, repo, "go", "/work/new", false); err != nil {
		t.Fatal(err)
	}
	if got, want := readString(t, fsys, "/work/new/skills-lock.json"), "{\"version\": 1}\n"; got != want {
		t.Errorf("lock = %q, want %q", got, want)
	}
	if got, want := run.Commands(), []string{"npx -y skills experimental_install"}; !slices.Equal(got, want) {
		t.Errorf("commands = %q, want %q", got, want)
	}
	if run.Calls[0].Dir != "/work/new" || run.Calls[0].Interactive {
		t.Errorf("cmd = %+v, want Dir /work/new, not interactive", run.Calls[0])
	}
}

func TestApplySkillsExisting(t *testing.T) {
	fsys := repoFS(t)
	if err := fsys.WriteFile("/work/skills-lock.json", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := &runner.FakeRunner{}
	err := ApplySkills(context.Background(), run, fsys, repo, "go", "/work", false)
	var ee *ExistsError
	if !errors.As(err, &ee) || ee.Path != "/work/skills-lock.json" {
		t.Fatalf("err = %v, want *ExistsError for /work/skills-lock.json", err)
	}
	if got := readString(t, fsys, "/work/skills-lock.json"); got != "old" {
		t.Errorf("lock changed to %q", got)
	}
	if len(run.Calls) != 0 {
		t.Errorf("ran %q", run.Commands())
	}

	if err := ApplySkills(context.Background(), run, fsys, repo, "rust", "/work", true); err != nil {
		t.Fatal(err)
	}
	if got, want := readString(t, fsys, "/work/skills-lock.json"), `{"version":1,"skills":{}}`; got != want {
		t.Errorf("lock = %q, want %q", got, want)
	}
	if len(run.Calls) != 1 {
		t.Errorf("commands = %q, want one", run.Commands())
	}
}

func TestApplySkillsRestoreFails(t *testing.T) {
	fsys := repoFS(t)
	want := &runner.CommandError{ExitCode: 1}
	run := &runner.FakeRunner{Script: map[string]runner.Result{"npx": {Err: want}}}
	err := ApplySkills(context.Background(), run, fsys, repo, "go", "/work", false)
	var ce *runner.CommandError
	if !errors.As(err, &ce) || ce != want {
		t.Errorf("err = %v, want the CommandError", err)
	}
}

func TestApplyAgents(t *testing.T) {
	fsys := repoFS(t)
	if err := ApplyAgents(fsys, repo, "go", "/work/new", false); err != nil {
		t.Fatal(err)
	}
	for dst, src := range map[string]string{
		"/work/new/AGENTS.md": "/repo/agents/go.md",
		"/work/new/CLAUDE.md": "/repo/agents/CLAUDE.md",
	} {
		got, err := fsys.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		want, err := fsys.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s = %q, want %q", dst, got, want)
		}
	}
}

func TestApplyAgentsExisting(t *testing.T) {
	for _, existing := range []string{"AGENTS.md", "CLAUDE.md"} {
		t.Run(existing, func(t *testing.T) {
			fsys := repoFS(t)
			path := "/work/" + existing
			if err := fsys.WriteFile(path, []byte("mine"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := ApplyAgents(fsys, repo, "rust", "/work", false)
			var ee *ExistsError
			if !errors.As(err, &ee) || ee.Path != path {
				t.Fatalf("err = %v, want *ExistsError for %s", err, path)
			}
			if got := readString(t, fsys, path); got != "mine" {
				t.Errorf("%s changed to %q", path, got)
			}
			for _, other := range []string{"/work/AGENTS.md", "/work/CLAUDE.md"} {
				if other == path {
					continue
				}
				if _, err := fsys.Stat(other); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("%s written despite refusal (stat err %v)", other, err)
				}
			}

			if err := ApplyAgents(fsys, repo, "rust", "/work", true); err != nil {
				t.Fatal(err)
			}
			if got := readString(t, fsys, "/work/AGENTS.md"); got != "# rust\n" {
				t.Errorf("AGENTS.md = %q", got)
			}
			if got := readString(t, fsys, "/work/CLAUDE.md"); got != "@AGENTS.md\n" {
				t.Errorf("CLAUDE.md = %q", got)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	msg := (&UnknownProfileError{Name: "x", Available: []string{"a", "b"}}).Error()
	if !strings.Contains(msg, `"x"`) || !strings.Contains(msg, "a, b") {
		t.Errorf("UnknownProfileError = %q", msg)
	}
	msg = (&UnknownTemplateError{Name: "y", Available: []string{"c"}}).Error()
	if !strings.Contains(msg, `"y"`) || !strings.Contains(msg, "c") {
		t.Errorf("UnknownTemplateError = %q", msg)
	}
	if msg := (&ExistsError{Path: "/p"}).Error(); !strings.Contains(msg, "/p") {
		t.Errorf("ExistsError = %q", msg)
	}
}
