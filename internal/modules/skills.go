package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

// skillsLockVersion is the only global lock version this module reads. It is
// CURRENT_VERSION in the vercel-labs skills CLI.
const skillsLockVersion = 3

// skillsSourceGitHub is the only source type that "skills add <owner/repo>"
// can restore.
const skillsSourceGitHub = "github"

// SkillsLockVersionError reports a skill lock file whose version this module
// does not know.
type SkillsLockVersionError struct {
	Path    string
	Version int
}

func (e *SkillsLockVersionError) Error() string {
	return fmt.Sprintf("skills: %s: unsupported lock version %d (want %d)", e.Path, e.Version, skillsLockVersion)
}

// SkillsSourceTypeError reports a desired skill whose source type cannot be
// restored with "skills add".
type SkillsSourceTypeError struct {
	Path       string
	Skill      string
	SourceType string
}

func (e *SkillsSourceTypeError) Error() string {
	return fmt.Sprintf("skills: %s: skill %q has source type %q (want %q)", e.Path, e.Skill, e.SourceType, skillsSourceGitHub)
}

// SkillsEntryError reports a lock entry that cannot become a safe command
// argument: an empty name or source, or one that starts with "-" and would
// be read as a flag.
type SkillsEntryError struct {
	Path   string
	Skill  string
	Reason string
}

func (e *SkillsEntryError) Error() string {
	return fmt.Sprintf("skills: %s: skill %q: %s", e.Path, e.Skill, e.Reason)
}

// skillsRawLock is the wire shape of a lock file. Fields that the module
// does not use (sourceUrl, hashes, timestamps, lastSelectedAgents, ...) are
// ignored by the decoder.
type skillsRawLock struct {
	Version *int                     `json:"version"`
	Skills  map[string]skillsRawItem `json:"skills"`
}

type skillsRawItem struct {
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
	SkillPath  string `json:"skillPath"`
}

// skillsEntry is one desired skill, parsed from the repo lock.
type skillsEntry struct {
	Name      string
	Source    string
	SkillPath string
}

// skillsGroup is the desired skills of one source, sorted by name.
type skillsGroup struct {
	Source string
	Skills []string
}

// skillsDecodeLock decodes a lock file and checks its version.
func skillsDecodeLock(path string, data []byte) (skillsRawLock, error) {
	var raw skillsRawLock
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return skillsRawLock{}, fmt.Errorf("skills: decode %s: %w", path, err)
	}
	if raw.Version == nil {
		return skillsRawLock{}, &SkillsLockVersionError{Path: path, Version: 0}
	}
	if *raw.Version != skillsLockVersion {
		return skillsRawLock{}, &SkillsLockVersionError{Path: path, Version: *raw.Version}
	}
	return raw, nil
}

// skillsParseDesired parses the repo lock into entries sorted by name.
func skillsParseDesired(path string, data []byte) ([]skillsEntry, error) {
	raw, err := skillsDecodeLock(path, data)
	if err != nil {
		return nil, err
	}
	entries := make([]skillsEntry, 0, len(raw.Skills))
	for name, item := range raw.Skills {
		if item.SourceType != skillsSourceGitHub {
			return nil, &SkillsSourceTypeError{Path: path, Skill: name, SourceType: item.SourceType}
		}
		if reason := skillsArgProblem(name, item.Source); reason != "" {
			return nil, &SkillsEntryError{Path: path, Skill: name, Reason: reason}
		}
		entries = append(entries, skillsEntry{Name: name, Source: item.Source, SkillPath: item.SkillPath})
	}
	slices.SortFunc(entries, func(a, b skillsEntry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

func skillsArgProblem(name, source string) string {
	switch {
	case name == "":
		return "empty skill name"
	case strings.HasPrefix(name, "-"):
		return "skill name starts with -"
	case source == "":
		return "empty source"
	case strings.HasPrefix(source, "-"):
		return "source starts with -"
	}
	return ""
}

// skillsParseInstalled returns the names in the installed global lock.
// Source types are not checked there: the user may have installed local or
// other skills by hand, and only the names matter for Check.
func skillsParseInstalled(path string, data []byte) (map[string]struct{}, error) {
	raw, err := skillsDecodeLock(path, data)
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(raw.Skills))
	for name := range raw.Skills {
		names[name] = struct{}{}
	}
	return names, nil
}

// skillsMissingGroups groups the desired skills that are not installed by
// source. Sources and skill names are sorted.
func skillsMissingGroups(desired []skillsEntry, installed map[string]struct{}) []skillsGroup {
	bySource := map[string][]string{}
	for _, e := range desired {
		if _, ok := installed[e.Name]; ok {
			continue
		}
		bySource[e.Source] = append(bySource[e.Source], e.Name)
	}
	groups := make([]skillsGroup, 0, len(bySource))
	for source, names := range bySource {
		slices.Sort(names)
		groups = append(groups, skillsGroup{Source: source, Skills: names})
	}
	slices.SortFunc(groups, func(a, b skillsGroup) int { return strings.Compare(a.Source, b.Source) })
	return groups
}

type skillsModule struct{}

// NewSkills returns the module that restores the global agent skills from
// skills/global.skill-lock.json with the vercel-labs skills CLI.
func NewSkills() module.Module { return skillsModule{} }

func (skillsModule) ID() module.ID { return "skills" }

func (skillsModule) Summary() string {
	return "restore global agent skills from skills/global.skill-lock.json"
}

func (skillsModule) DependsOn() []module.ID { return []module.ID{"repo", "mise"} }

func (skillsModule) Supports(platform.Platform) bool { return true }

func (skillsModule) Check(_ context.Context, env module.Env) (module.Status, error) {
	groups, err := skillsPlan(env)
	if err != nil {
		return module.StatusMissing, err
	}
	if len(groups) == 0 {
		return module.StatusInstalled, nil
	}
	return module.StatusMissing, nil
}

func (skillsModule) Apply(ctx context.Context, env module.Env) error {
	groups, err := skillsPlan(env)
	if err != nil {
		return err
	}
	for _, g := range groups {
		args := []string{"-y", "skills", "add", g.Source, "--global"}
		for _, name := range g.Skills {
			args = append(args, "--skill", name)
		}
		args = append(args, "--yes")
		if err := env.Run.Run(ctx, runner.Cmd{Name: "npx", Args: args, Dir: env.Home}); err != nil {
			return fmt.Errorf("skills: install from %s: %w", g.Source, err)
		}
	}
	return nil
}

// skillsPlan reads the desired and installed locks and returns the installs
// that are still needed.
func skillsPlan(env module.Env) ([]skillsGroup, error) {
	desiredPath := filepath.Join(env.RepoDir, "skills", "global.skill-lock.json")
	data, err := env.FS.ReadFile(desiredPath)
	if err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", desiredPath, err)
	}
	desired, err := skillsParseDesired(desiredPath, data)
	if err != nil {
		return nil, err
	}

	installedPath := filepath.Join(env.Home, ".agents", ".skill-lock.json")
	installed := map[string]struct{}{}
	data, err = env.FS.ReadFile(installedPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("skills: read %s: %w", installedPath, err)
	default:
		installed, err = skillsParseInstalled(installedPath, data)
		if err != nil {
			return nil, err
		}
	}
	return skillsMissingGroups(desired, installed), nil
}
