// Package project applies repo data to a project directory: a skills profile
// (dot skills apply) and an AGENTS.md template (dot agents apply).
//
// cmd/dot parses the user's profile or template name with ParseProfile or
// ParseTemplate, then passes the result to ApplySkills or ApplyAgents.
package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// FS is the file access this package needs: a module.WriteFS that can also
// list a directory, so the parse functions can report the names that exist.
type FS interface {
	module.WriteFS
	ReadDir(name string) ([]fs.DirEntry, error)
}

// OSFS is the FS of the real file system.
type OSFS struct {
	module.OSFS
}

var _ FS = OSFS{}

// ReadDir calls os.ReadDir.
func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

// ProfileName names a skills profile, skills/profiles/<name>.json. A
// ProfileName is valid only after ParseProfile accepted it.
type ProfileName string

// TemplateName names an agents template, agents/<name>.md. A TemplateName is
// valid only after ParseTemplate accepted it.
type TemplateName string

// UnknownProfileError reports a profile name with no file in skills/profiles.
type UnknownProfileError struct {
	Name string
	// Available lists the profile names that exist, sorted.
	Available []string
}

func (e *UnknownProfileError) Error() string {
	return fmt.Sprintf("unknown skills profile %q (available: %s)", e.Name, strings.Join(e.Available, ", "))
}

// UnknownTemplateError reports a template name with no file in agents.
type UnknownTemplateError struct {
	Name string
	// Available lists the template names that exist, sorted.
	Available []string
}

func (e *UnknownTemplateError) Error() string {
	return fmt.Sprintf("unknown agents template %q (available: %s)", e.Name, strings.Join(e.Available, ", "))
}

// ExistsError reports a target file that already exists and was not
// overwritten because force was not set.
type ExistsError struct {
	Path string
}

func (e *ExistsError) Error() string {
	return fmt.Sprintf("%s already exists", e.Path)
}

const (
	lockFile   = "skills-lock.json"
	agentsFile = "AGENTS.md"
	claudeFile = "CLAUDE.md"
	fileMode   = 0o644
	dirMode    = 0o755
)

// nonTemplates are the files in agents/ that are not AGENTS.md templates.
var nonTemplates = []string{"README.md", claudeFile}

func profilesDir(repoDir string) string { return filepath.Join(repoDir, "skills", "profiles") }

func agentsDir(repoDir string) string { return filepath.Join(repoDir, "agents") }

// ParseProfile returns name as a ProfileName when
// <repoDir>/skills/profiles/<name>.json exists. Otherwise it returns
// *UnknownProfileError with the available names.
func ParseProfile(fsys FS, repoDir, name string) (ProfileName, error) {
	names, err := listNames(fsys, profilesDir(repoDir), ".json", nil)
	if err != nil {
		return "", err
	}
	if !slices.Contains(names, name) {
		return "", &UnknownProfileError{Name: name, Available: names}
	}
	return ProfileName(name), nil
}

// ParseTemplate returns name as a TemplateName when <repoDir>/agents/<name>.md
// exists and is a template (not README.md or CLAUDE.md). Otherwise it returns
// *UnknownTemplateError with the available names.
func ParseTemplate(fsys FS, repoDir, name string) (TemplateName, error) {
	names, err := listNames(fsys, agentsDir(repoDir), ".md", nonTemplates)
	if err != nil {
		return "", err
	}
	if !slices.Contains(names, name) {
		return "", &UnknownTemplateError{Name: name, Available: names}
	}
	return TemplateName(name), nil
}

// listNames returns the sorted base names, without ext, of the regular files
// in dir that end in ext and are not in exclude.
func listNames(fsys FS, dir, ext string, exclude []string) ([]string, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(n, ext) || slices.Contains(exclude, n) {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ext))
	}
	slices.Sort(names)
	return names, nil
}

// SkillsRestoreCmd is the skills CLI command that installs every skill in
// dir/skills-lock.json into the project dir. The CLI reads the lock file from
// its working directory and answers its own prompts.
func SkillsRestoreCmd(dir string) runner.Cmd {
	return runner.Cmd{Name: "npx", Args: []string{"-y", "skills", "experimental_install"}, Dir: dir}
}

// ApplySkills copies skills/profiles/<p>.json to <targetDir>/skills-lock.json
// and then runs SkillsRestoreCmd in targetDir. When the lock file exists and
// force is false, it returns *ExistsError and changes nothing.
func ApplySkills(ctx context.Context, run runner.Runner, fsys FS, repoDir string, p ProfileName, targetDir string, force bool) error {
	dst := filepath.Join(targetDir, lockFile)
	if !force {
		if err := refuseExisting(fsys, dst); err != nil {
			return err
		}
	}
	data, err := fsys.ReadFile(filepath.Join(profilesDir(repoDir), string(p)+".json"))
	if err != nil {
		return fmt.Errorf("read skills profile %s: %w", p, err)
	}
	if err := writeFile(fsys, dst, data); err != nil {
		return err
	}
	return run.Run(ctx, SkillsRestoreCmd(targetDir))
}

// ApplyAgents writes agents/<t>.md to <targetDir>/AGENTS.md and
// agents/CLAUDE.md to <targetDir>/CLAUDE.md. When force is false and either
// target exists, it returns *ExistsError and writes neither.
func ApplyAgents(fsys FS, repoDir string, t TemplateName, targetDir string, force bool) error {
	copies := []struct{ src, dst string }{
		{filepath.Join(agentsDir(repoDir), string(t)+".md"), filepath.Join(targetDir, agentsFile)},
		{filepath.Join(agentsDir(repoDir), claudeFile), filepath.Join(targetDir, claudeFile)},
	}
	if !force {
		for _, c := range copies {
			if err := refuseExisting(fsys, c.dst); err != nil {
				return err
			}
		}
	}
	contents := make([][]byte, len(copies))
	for i, c := range copies {
		data, err := fsys.ReadFile(c.src)
		if err != nil {
			return fmt.Errorf("read agents template: %w", err)
		}
		contents[i] = data
	}
	for i, c := range copies {
		if err := writeFile(fsys, c.dst, contents[i]); err != nil {
			return err
		}
	}
	return nil
}

// refuseExisting returns *ExistsError when path exists, nil when it does not,
// and any other Stat error as is.
func refuseExisting(fsys FS, path string) error {
	_, err := fsys.Stat(path)
	switch {
	case err == nil:
		return &ExistsError{Path: path}
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("check %s: %w", path, err)
	}
}

func writeFile(fsys FS, path string, data []byte) error {
	if err := fsys.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := fsys.WriteFile(path, data, fileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
