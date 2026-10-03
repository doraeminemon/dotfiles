package modules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
)

// Secret is one value that the secrets module asks for and writes to
// secrets.fish as a global exported fish variable.
type Secret struct {
	// EnvVar is the variable name. It matches ^[A-Z_][A-Z0-9_]*$.
	EnvVar string
	// Prompt is the question shown to the user.
	Prompt string
}

// SecretsMissingError reports a secret that could not be asked for (no TTY)
// and whose fallback environment variable is not set. It never holds a value.
type SecretsMissingError struct {
	EnvVar      string
	FallbackVar string
}

func (e *SecretsMissingError) Error() string {
	return fmt.Sprintf("secrets: %s: no terminal for a masked prompt and %s is not set", e.EnvVar, e.FallbackVar)
}

// secretsList is the fixed set of secrets. Every EnvVar must match
// secretsEnvVarPattern; a test asserts it.
var secretsList = []Secret{
	{EnvVar: "EXA_API_KEY", Prompt: "Exa API key (https://dashboard.exa.ai)"},
}

const (
	secretsHeader     = "# Written by dot. Not tracked in git."
	secretsFallbackPx = "DOT_"
	secretsFileMode   = fs.FileMode(0o600)
	secretsDirMode    = fs.FileMode(0o700)
)

var (
	secretsEnvVarPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	secretsLinePattern   = regexp.MustCompile(`^set -gx ([A-Z_][A-Z0-9_]*) '(.*)'$`)

	errSecretsEmpty     = errors.New("value must not be empty")
	errSecretsMultiline = errors.New("value must be a single line")
)

type secretsModule struct {
	lookupEnv func(string) (string, bool)
}

// NewSecrets returns the module that writes API keys to
// ~/.config/fish/conf.d/secrets.fish with mode 0600. lookupEnv reads the
// DOT_<VAR> fallback when no terminal is available; the CLI passes
// os.LookupEnv.
func NewSecrets(lookupEnv func(string) (string, bool)) module.Module {
	return secretsModule{lookupEnv: lookupEnv}
}

func (secretsModule) ID() module.ID { return "secrets" }

func (secretsModule) Summary() string {
	return "Write API keys to ~/.config/fish/conf.d/secrets.fish (mode 0600)"
}

func (secretsModule) DependsOn() []module.ID { return nil }

func (secretsModule) Supports(platform.Platform) bool { return true }

func (secretsModule) Check(_ context.Context, env module.Env) (module.Status, error) {
	f, err := secretsRead(env)
	if err != nil {
		return module.StatusMissing, err
	}
	for _, s := range secretsList {
		if _, ok := f.get(s.EnvVar); !ok {
			return module.StatusMissing, nil
		}
	}
	return module.StatusInstalled, nil
}

func (m secretsModule) Apply(_ context.Context, env module.Env) error {
	f, err := secretsRead(env)
	if err != nil {
		return err
	}
	changed := false
	for _, s := range secretsList {
		if _, ok := f.get(s.EnvVar); ok {
			if env.Yes {
				continue
			}
			replace, err := env.Ask.Confirm("Replace "+s.EnvVar+"?", false)
			if err != nil {
				return fmt.Errorf("secrets: confirm %s: %w", s.EnvVar, err)
			}
			if !replace {
				continue
			}
		}
		value, err := m.value(env, s)
		if err != nil {
			return err
		}
		f.set(s.EnvVar, value)
		changed = true
	}
	if !changed {
		return nil
	}
	return secretsWrite(env, f)
}

// value asks for s with masked input, or reads DOT_<VAR> when no TTY exists.
func (m secretsModule) value(env module.Env, s Secret) (string, error) {
	v, err := env.Ask.Input(s.Prompt, prompt.InputOpts{Secret: true, Validate: secretsValidate})
	if err == nil {
		return v, nil
	}
	var tty *prompt.SecretNeedsTTYError
	if !errors.As(err, &tty) {
		return "", fmt.Errorf("secrets: ask %s: %w", s.EnvVar, err)
	}
	fallback := secretsFallbackPx + s.EnvVar
	v, ok := m.lookupEnv(fallback)
	if !ok || v == "" {
		return "", &SecretsMissingError{EnvVar: s.EnvVar, FallbackVar: fallback}
	}
	if err := secretsValidate(v); err != nil {
		return "", fmt.Errorf("secrets: %s: %w", fallback, err)
	}
	return v, nil
}

// secretsValidate rejects values that cannot be stored on one line. Its
// errors never contain the value.
func secretsValidate(v string) error {
	if v == "" {
		return errSecretsEmpty
	}
	if strings.ContainsAny(v, "\r\n") {
		return errSecretsMultiline
	}
	return nil
}

func secretsPath(env module.Env) string {
	return filepath.Join(env.Home, ".config", "fish", "conf.d", "secrets.fish")
}

// secretsLine is one line of secrets.fish: a parsed assignment when name is
// set, or a line kept verbatim in raw.
type secretsLine struct {
	name  string
	value string
	raw   string
}

type secretsFile struct {
	lines []secretsLine
}

func (f *secretsFile) get(name string) (string, bool) {
	for _, l := range f.lines {
		if l.name == name {
			return l.value, true
		}
	}
	return "", false
}

func (f *secretsFile) set(name, value string) {
	for i := range f.lines {
		if f.lines[i].name == name {
			f.lines[i].value = value
			return
		}
	}
	f.lines = append(f.lines, secretsLine{name: name, value: value})
}

func (f *secretsFile) render() []byte {
	var b strings.Builder
	for _, l := range f.lines {
		if l.name == "" {
			b.WriteString(l.raw)
		} else {
			b.WriteString("set -gx " + l.name + " " + secretsQuote(l.value))
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// secretsParse reads the file. An empty file gets the header. Lines that are not exactly
// `set -gx NAME 'value'` are kept verbatim.
func secretsParse(data string) *secretsFile {
	data = strings.TrimSuffix(data, "\n")
	if data == "" {
		return &secretsFile{lines: []secretsLine{{raw: secretsHeader}}}
	}
	f := &secretsFile{}
	for _, raw := range strings.Split(data, "\n") {
		if m := secretsLinePattern.FindStringSubmatch(raw); m != nil {
			if v, ok := secretsUnquote(m[2]); ok {
				f.lines = append(f.lines, secretsLine{name: m[1], value: v})
				continue
			}
		}
		f.lines = append(f.lines, secretsLine{raw: raw})
	}
	return f
}

// secretsQuote returns v as a fish single-quoted string. Inside single
// quotes fish treats only \\ and \' as escapes.
func secretsQuote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(v) + "'"
}

// secretsUnquote reverses secretsQuote on the text between the quotes. It
// fails on an unescaped quote, which means the line is not one we wrote.
func secretsUnquote(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			return "", false
		case c == '\\' && i+1 < len(s) && (s[i+1] == '\\' || s[i+1] == '\''):
			b.WriteByte(s[i+1])
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}

func secretsRead(env module.Env) (*secretsFile, error) {
	path := secretsPath(env)
	data, err := env.FS.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return secretsParse(""), nil
	}
	if err != nil {
		return nil, fmt.Errorf("secrets: read %s: %w", path, err)
	}
	return secretsParse(string(data)), nil
}

// secretsWrite replaces the file atomically: write a 0600 temp file in the
// same directory, then rename it over the target.
func secretsWrite(env module.Env, f *secretsFile) error {
	path := secretsPath(env)
	if err := env.FS.MkdirAll(filepath.Dir(path), secretsDirMode); err != nil {
		return fmt.Errorf("secrets: create %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	// os.WriteFile keeps the mode of an existing file, so a stale tmp file
	// with a wider mode must go first.
	if err := env.FS.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("secrets: remove stale %s: %w", tmp, err)
	}
	if err := env.FS.WriteFile(tmp, f.render(), secretsFileMode); err != nil {
		return fmt.Errorf("secrets: write %s: %w", tmp, err)
	}
	if err := env.FS.Rename(tmp, path); err != nil {
		return fmt.Errorf("secrets: rename %s: %w", tmp, err)
	}
	return nil
}
