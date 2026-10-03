package modules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	skillsTestHome = "/home/u"
	skillsTestRepo = "/home/u/Projects/dotfiles"
)

var skillsTestPlatforms = []platform.Platform{
	{OS: platform.Darwin, Arch: platform.ARM64},
	{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt},
}

// skillsTestLock is a small desired lock with two sources and extra fields
// that the parser must ignore.
const skillsTestLock = `{
  "version": 3,
  "skills": {
    "b-skill": {"source": "org/two", "sourceType": "github", "sourceUrl": "https://github.com/org/two.git", "skillPath": "skills/b/SKILL.md", "skillFolderHash": "x"},
    "a-skill": {"source": "org/two", "sourceType": "github", "skillPath": "skills/a/SKILL.md"},
    "c-skill": {"source": "org/one", "sourceType": "github", "skillPath": "skills/c/SKILL.md", "pluginName": "p"}
  },
  "dismissed": {"findSkillsPrompt": true},
  "lastSelectedAgents": ["claude-code"]
}`

func newSkillsTestEnv(t *testing.T, p platform.Platform, desired, installed string) (module.Env, *runner.FakeRunner) {
	t.Helper()
	fsys := &module.MemFS{}
	skillsWrite(t, fsys, filepath.Join(skillsTestRepo, "skills", "global.skill-lock.json"), desired)
	if installed != "" {
		skillsWrite(t, fsys, filepath.Join(skillsTestHome, ".agents", ".skill-lock.json"), installed)
	} else if err := fsys.MkdirAll(skillsTestHome, 0o755); err != nil {
		t.Fatal(err)
	}
	run := &runner.FakeRunner{}
	return module.Env{
		Platform: p,
		Run:      run,
		Ask:      &prompt.FakePrompter{},
		FS:       fsys,
		RepoDir:  skillsTestRepo,
		Home:     skillsTestHome,
	}, run
}

func skillsWrite(t *testing.T, fsys *module.MemFS, path, data string) {
	t.Helper()
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillsMetadata(t *testing.T) {
	m := NewSkills()
	if m.ID() != "skills" {
		t.Errorf("ID = %q", m.ID())
	}
	if got, want := m.DependsOn(), []module.ID{"repo", "mise"}; !slices.Equal(got, want) {
		t.Errorf("DependsOn = %v, want %v", got, want)
	}
	for _, p := range skillsTestPlatforms {
		if !m.Supports(p) {
			t.Errorf("Supports(%v) = false", p)
		}
	}
}

func TestSkillsParseRealLock(t *testing.T) {
	data, err := os.ReadFile("../../skills/global.skill-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := skillsParseDesired("global.skill-lock.json", data)
	if err != nil {
		t.Fatalf("parse real lock: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	byName := map[string]skillsEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	want := skillsEntry{Name: "find-skills", Source: "vercel-labs/skills", SkillPath: "skills/find-skills/SKILL.md"}
	if got := byName["find-skills"]; got != want {
		t.Errorf("find-skills = %+v, want %+v", got, want)
	}
	if !slices.IsSortedFunc(entries, func(a, b skillsEntry) int { return strings.Compare(a.Name, b.Name) }) {
		t.Error("entries are not sorted by name")
	}

	// One install call per distinct source.
	sources := map[string]struct{}{}
	for _, e := range entries {
		sources[e.Source] = struct{}{}
	}
	env, run := newSkillsTestEnv(t, skillsTestPlatforms[0], string(data), "")
	if err := NewSkills().Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if got := len(run.Commands()); got != len(sources) {
		t.Errorf("Apply ran %d commands, want one per source (%d): %v", got, len(sources), run.Commands())
	}
}

func TestSkillsApplyFresh(t *testing.T) {
	for _, p := range skillsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			env, run := newSkillsTestEnv(t, p, skillsTestLock, "")
			st, err := NewSkills().Check(context.Background(), env)
			if err != nil || st != module.StatusMissing {
				t.Fatalf("Check = %v, %v; want missing", st, err)
			}
			if len(run.Calls) != 0 {
				t.Fatalf("Check ran commands: %v", run.Commands())
			}
			if err := NewSkills().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			want := []string{
				"npx -y skills add org/one --global --skill c-skill --yes",
				"npx -y skills add org/two --global --skill a-skill --skill b-skill --yes",
			}
			if got := run.Commands(); !slices.Equal(got, want) {
				t.Errorf("commands:\n got %q\nwant %q", got, want)
			}
			for _, c := range run.Calls {
				if c.Dir != skillsTestHome || c.Interactive {
					t.Errorf("cmd %v: Dir=%q Interactive=%v", c, c.Dir, c.Interactive)
				}
			}
		})
	}
}

func TestSkillsPartiallyInstalled(t *testing.T) {
	installed := `{"version": 3, "skills": {"c-skill": {"source": "org/one", "sourceType": "github"}, "a-skill": {"source": "org/two", "sourceType": "github"}, "mine": {"source": "/x", "sourceType": "local"}}}`
	for _, p := range skillsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			env, run := newSkillsTestEnv(t, p, skillsTestLock, installed)
			st, err := NewSkills().Check(context.Background(), env)
			if err != nil || st != module.StatusMissing {
				t.Fatalf("Check = %v, %v; want missing", st, err)
			}
			if err := NewSkills().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			want := []string{"npx -y skills add org/two --global --skill b-skill --yes"}
			if got := run.Commands(); !slices.Equal(got, want) {
				t.Errorf("commands:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestSkillsAllInstalled(t *testing.T) {
	for _, p := range skillsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			env, run := newSkillsTestEnv(t, p, skillsTestLock, skillsTestLock)
			st, err := NewSkills().Check(context.Background(), env)
			if err != nil || st != module.StatusInstalled {
				t.Fatalf("Check = %v, %v; want installed", st, err)
			}
			if err := NewSkills().Apply(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			if len(run.Calls) != 0 {
				t.Errorf("Apply ran commands: %v", run.Commands())
			}
		})
	}
}

func TestSkillsApplyCommandError(t *testing.T) {
	env, run := newSkillsTestEnv(t, skillsTestPlatforms[1], skillsTestLock, "")
	cmdErr := &runner.CommandError{ExitCode: 1}
	run.Script = map[string]runner.Result{"npx": {Err: cmdErr}}
	err := NewSkills().Apply(context.Background(), env)
	var got *runner.CommandError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v, want *runner.CommandError", err)
	}
	if len(run.Calls) != 1 {
		t.Errorf("Apply continued after a failure: %v", run.Commands())
	}
}

func TestSkillsLockVersionError(t *testing.T) {
	for _, tc := range []struct {
		name, desired, installed string
		wantPath                 string
		wantVersion              int
	}{
		{"desired v2", `{"version": 2, "skills": {}}`, "", "global.skill-lock.json", 2},
		{"desired no version", `{"skills": {}}`, "", "global.skill-lock.json", 0},
		{"installed v4", skillsTestLock, `{"version": 4, "skills": {}}`, ".skill-lock.json", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, run := newSkillsTestEnv(t, skillsTestPlatforms[0], tc.desired, tc.installed)
			_, err := NewSkills().Check(context.Background(), env)
			var verr *SkillsLockVersionError
			if !errors.As(err, &verr) {
				t.Fatalf("Check err = %v, want *SkillsLockVersionError", err)
			}
			if verr.Version != tc.wantVersion || filepath.Base(verr.Path) != tc.wantPath {
				t.Errorf("err = %+v", verr)
			}
			if err := NewSkills().Apply(context.Background(), env); !errors.As(err, &verr) {
				t.Errorf("Apply err = %v, want *SkillsLockVersionError", err)
			}
			if len(run.Calls) != 0 {
				t.Errorf("ran commands: %v", run.Commands())
			}
		})
	}
}

func TestSkillsSourceTypeError(t *testing.T) {
	desired := `{"version": 3, "skills": {"x": {"source": "./local", "sourceType": "local"}}}`
	env, run := newSkillsTestEnv(t, skillsTestPlatforms[1], desired, "")
	err := NewSkills().Apply(context.Background(), env)
	var serr *SkillsSourceTypeError
	if !errors.As(err, &serr) {
		t.Fatalf("err = %v, want *SkillsSourceTypeError", err)
	}
	if serr.Skill != "x" || serr.SourceType != "local" {
		t.Errorf("err = %+v", serr)
	}
	if len(run.Calls) != 0 {
		t.Errorf("ran commands: %v", run.Commands())
	}
}

func TestSkillsEntryError(t *testing.T) {
	for _, desired := range []string{
		`{"version": 3, "skills": {"--all": {"source": "org/a", "sourceType": "github"}}}`,
		`{"version": 3, "skills": {"x": {"source": "", "sourceType": "github"}}}`,
		`{"version": 3, "skills": {"x": {"source": "-g", "sourceType": "github"}}}`,
	} {
		env, _ := newSkillsTestEnv(t, skillsTestPlatforms[0], desired, "")
		_, err := NewSkills().Check(context.Background(), env)
		var eerr *SkillsEntryError
		if !errors.As(err, &eerr) {
			t.Errorf("%s: err = %v, want *SkillsEntryError", desired, err)
		}
	}
}

func TestSkillsMalformedAndMissing(t *testing.T) {
	env, _ := newSkillsTestEnv(t, skillsTestPlatforms[0], `{not json`, "")
	if _, err := NewSkills().Check(context.Background(), env); err == nil {
		t.Error("malformed lock: want error")
	}

	env.FS = &module.MemFS{}
	_, err := NewSkills().Check(context.Background(), env)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing repo lock: err = %v, want fs.ErrNotExist", err)
	}
}
