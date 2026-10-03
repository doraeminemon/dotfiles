package modules

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/doraeminemon/dotfiles/internal/module"
	"github.com/doraeminemon/dotfiles/internal/platform"
	"github.com/doraeminemon/dotfiles/internal/prompt"
	"github.com/doraeminemon/dotfiles/internal/runner"
)

const (
	secretsTestHome = "/home/tester"
	secretsTestPath = secretsTestHome + "/.config/fish/conf.d/secrets.fish"
)

var secretsTestPlatforms = []platform.Platform{
	{OS: platform.Darwin, Arch: platform.ARM64},
	{OS: platform.Linux, Arch: platform.AMD64, PkgMgr: platform.Apt},
}

type secretsTestEnv struct {
	env module.Env
	fs  *module.MemFS
	run *runner.FakeRunner
}

func newSecretsTestEnv(t *testing.T, p platform.Platform, ask prompt.Prompter) secretsTestEnv {
	t.Helper()
	mfs := &module.MemFS{}
	if err := mfs.MkdirAll(secretsTestHome, 0o755); err != nil {
		t.Fatal(err)
	}
	run := &runner.FakeRunner{}
	return secretsTestEnv{
		env: module.Env{Platform: p, Run: run, Ask: ask, FS: mfs, Home: secretsTestHome, RepoDir: "/repo"},
		fs:  mfs,
		run: run,
	}
}

func secretsNoEnv(string) (string, bool) { return "", false }

// secretsTTYPrompter is a Prompter with no terminal: every secret Input
// fails with *prompt.SecretNeedsTTYError, as HuhPrompter does.
type secretsTTYPrompter struct {
	prompt.FakePrompter
}

func (p *secretsTTYPrompter) Input(title string, opts prompt.InputOpts) (string, error) {
	p.Asked = append(p.Asked, title)
	if opts.Secret {
		return "", &prompt.SecretNeedsTTYError{Title: title}
	}
	return p.FakePrompter.Input(title, opts)
}

func secretsSeed(t *testing.T, e secretsTestEnv, content string) {
	t.Helper()
	if err := e.fs.MkdirAll(secretsTestHome+"/.config/fish/conf.d", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := e.fs.WriteFile(secretsTestPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func secretsReadFile(t *testing.T, e secretsTestEnv) string {
	t.Helper()
	data, err := e.fs.ReadFile(secretsTestPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func secretsAssertNoLeak(t *testing.T, e secretsTestEnv, asked []string, err error, value string) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), value) {
		t.Errorf("error leaks the secret value: %v", err)
	}
	for _, a := range asked {
		if strings.Contains(a, value) {
			t.Errorf("prompt title leaks the secret value: %q", a)
		}
	}
	for _, c := range e.run.Commands() {
		if strings.Contains(c, value) {
			t.Errorf("runner command leaks the secret value: %q", c)
		}
	}
}

func TestSecretsEnvVarNames(t *testing.T) {
	if len(secretsList) == 0 {
		t.Fatal("secretsList is empty")
	}
	for _, s := range secretsList {
		if !secretsEnvVarPattern.MatchString(s.EnvVar) {
			t.Errorf("EnvVar %q does not match %s", s.EnvVar, secretsEnvVarPattern)
		}
		if s.Prompt == "" {
			t.Errorf("EnvVar %q has no prompt", s.EnvVar)
		}
	}
}

func TestSecretsMetadata(t *testing.T) {
	m := NewSecrets(secretsNoEnv)
	if m.ID() != "secrets" {
		t.Errorf("ID = %q", m.ID())
	}
	if len(m.DependsOn()) != 0 {
		t.Errorf("DependsOn = %v, want none", m.DependsOn())
	}
	for _, p := range secretsTestPlatforms {
		if !m.Supports(p) {
			t.Errorf("Supports(%v) = false", p)
		}
	}
}

func TestSecretsApplyWritesFileMode0600(t *testing.T) {
	for _, p := range secretsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			const value = "exa-secret-value-123"
			ask := &prompt.FakePrompter{Inputs: []string{value}}
			e := newSecretsTestEnv(t, p, ask)
			m := NewSecrets(secretsNoEnv)
			ctx := context.Background()

			st, err := m.Check(ctx, e.env)
			if err != nil || st != module.StatusMissing {
				t.Fatalf("Check before = %v, %v; want missing", st, err)
			}
			err = m.Apply(ctx, e.env)
			secretsAssertNoLeak(t, e, ask.Asked, err, value)
			if err != nil {
				t.Fatal(err)
			}

			info, err := e.fs.Stat(secretsTestPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("file mode = %o, want 600", info.Mode().Perm())
			}
			dir, err := e.fs.Stat(secretsTestHome + "/.config/fish/conf.d")
			if err != nil {
				t.Fatal(err)
			}
			if dir.Mode().Perm() != 0o700 {
				t.Errorf("dir mode = %o, want 700", dir.Mode().Perm())
			}
			if _, err := e.fs.Stat(secretsTestPath + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("temp file left behind: %v", err)
			}
			want := secretsHeader + "\nset -gx EXA_API_KEY '" + value + "'\n"
			if got := secretsReadFile(t, e); got != want {
				t.Errorf("file =\n%s\nwant\n%s", got, want)
			}
			if len(ask.Asked) != 1 || ask.Asked[0] != secretsList[0].Prompt {
				t.Errorf("Asked = %q", ask.Asked)
			}

			st, err = m.Check(ctx, e.env)
			if err != nil || st != module.StatusInstalled {
				t.Errorf("Check after = %v, %v; want installed", st, err)
			}
		})
	}
}

func TestSecretsQuoteRoundTrip(t *testing.T) {
	cases := []string{
		`plain`,
		`it's`,
		`back\slash`,
		`trailing\`,
		`\'`,
		`''\\''`,
		`$HOME (cmd) "dq" *glob ~`,
	}
	for _, v := range cases {
		line := "set -gx EXA_API_KEY " + secretsQuote(v)
		f := secretsParse(line + "\n")
		got, ok := f.get("EXA_API_KEY")
		if !ok || got != v {
			t.Errorf("round trip of %q via %q = %q, %v", v, line, got, ok)
		}
	}
	if got := secretsQuote(`a'b\c`); got != `'a\'b\\c'` {
		t.Errorf("secretsQuote = %s", got)
	}
}

func TestSecretsApplyEscapesQuotes(t *testing.T) {
	for _, p := range secretsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			const value = `ab'c\d`
			e := newSecretsTestEnv(t, p, &prompt.FakePrompter{Inputs: []string{value}})
			if err := NewSecrets(secretsNoEnv).Apply(context.Background(), e.env); err != nil {
				t.Fatal(err)
			}
			got := secretsReadFile(t, e)
			if !strings.Contains(got, `set -gx EXA_API_KEY 'ab\'c\\d'`+"\n") {
				t.Errorf("file = %q", got)
			}
			v, ok := secretsParse(got).get("EXA_API_KEY")
			if !ok || v != value {
				t.Errorf("parsed back %q, %v", v, ok)
			}
		})
	}
}

func TestSecretsKeepExisting(t *testing.T) {
	const existing = "# my note\nset -gx OTHER 'x'\nset -gx EXA_API_KEY 'old-value'\nfunction f; end\n"
	for _, p := range secretsTestPlatforms {
		t.Run(p.String()+"/declined", func(t *testing.T) {
			ask := &prompt.FakePrompter{Confirms: []bool{false}}
			e := newSecretsTestEnv(t, p, ask)
			secretsSeed(t, e, existing)
			m := NewSecrets(secretsNoEnv)
			st, err := m.Check(context.Background(), e.env)
			if err != nil || st != module.StatusInstalled {
				t.Fatalf("Check = %v, %v; want installed", st, err)
			}
			if err := m.Apply(context.Background(), e.env); err != nil {
				t.Fatal(err)
			}
			if got := secretsReadFile(t, e); got != existing {
				t.Errorf("file changed:\n%s", got)
			}
			if len(ask.Asked) != 1 || ask.Asked[0] != "Replace EXA_API_KEY?" {
				t.Errorf("Asked = %q", ask.Asked)
			}
		})
		t.Run(p.String()+"/yes", func(t *testing.T) {
			ask := &prompt.FakePrompter{}
			e := newSecretsTestEnv(t, p, ask)
			e.env.Yes = true
			secretsSeed(t, e, existing)
			if err := NewSecrets(secretsNoEnv).Apply(context.Background(), e.env); err != nil {
				t.Fatal(err)
			}
			if got := secretsReadFile(t, e); got != existing {
				t.Errorf("file changed:\n%s", got)
			}
			if len(ask.Asked) != 0 {
				t.Errorf("Asked = %q, want nothing", ask.Asked)
			}
		})
		t.Run(p.String()+"/replace", func(t *testing.T) {
			const value = "new-value"
			ask := &prompt.FakePrompter{Confirms: []bool{true}, Inputs: []string{value}}
			e := newSecretsTestEnv(t, p, ask)
			secretsSeed(t, e, existing)
			err := NewSecrets(secretsNoEnv).Apply(context.Background(), e.env)
			secretsAssertNoLeak(t, e, ask.Asked, err, value)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(existing, "old-value", value, 1)
			if got := secretsReadFile(t, e); got != want {
				t.Errorf("file =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestSecretsEnvFallback(t *testing.T) {
	for _, p := range secretsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			const value = "from-env-value"
			var looked []string
			lookup := func(k string) (string, bool) {
				looked = append(looked, k)
				if k == "DOT_EXA_API_KEY" {
					return value, true
				}
				return "", false
			}
			ask := &secretsTTYPrompter{}
			e := newSecretsTestEnv(t, p, ask)
			err := NewSecrets(lookup).Apply(context.Background(), e.env)
			secretsAssertNoLeak(t, e, ask.Asked, err, value)
			if err != nil {
				t.Fatal(err)
			}
			if len(looked) != 1 || looked[0] != "DOT_EXA_API_KEY" {
				t.Errorf("looked up %q", looked)
			}
			if got := secretsReadFile(t, e); !strings.Contains(got, "set -gx EXA_API_KEY '"+value+"'\n") {
				t.Errorf("file = %q", got)
			}
		})
	}
}

func TestSecretsMissingFallback(t *testing.T) {
	for _, p := range secretsTestPlatforms {
		t.Run(p.String(), func(t *testing.T) {
			e := newSecretsTestEnv(t, p, &secretsTTYPrompter{})
			err := NewSecrets(secretsNoEnv).Apply(context.Background(), e.env)
			var missing *SecretsMissingError
			if !errors.As(err, &missing) {
				t.Fatalf("err = %v, want *SecretsMissingError", err)
			}
			if missing.EnvVar != "EXA_API_KEY" || missing.FallbackVar != "DOT_EXA_API_KEY" {
				t.Errorf("err = %+v", missing)
			}
			if _, err := e.fs.Stat(secretsTestPath); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("file written despite error: %v", err)
			}
		})
	}
}

func TestSecretsFallbackRejectsMultiline(t *testing.T) {
	const value = "line1\nline2-secret"
	lookup := func(string) (string, bool) { return value, true }
	ask := &secretsTTYPrompter{}
	e := newSecretsTestEnv(t, secretsTestPlatforms[0], ask)
	err := NewSecrets(lookup).Apply(context.Background(), e.env)
	if !errors.Is(err, errSecretsMultiline) {
		t.Fatalf("err = %v, want errSecretsMultiline", err)
	}
	secretsAssertNoLeak(t, e, ask.Asked, err, "line2-secret")
}

func TestSecretsInputValidateRejectsEmpty(t *testing.T) {
	ask := &prompt.FakePrompter{Inputs: []string{""}}
	e := newSecretsTestEnv(t, secretsTestPlatforms[1], ask)
	err := NewSecrets(secretsNoEnv).Apply(context.Background(), e.env)
	if !errors.Is(err, errSecretsEmpty) {
		t.Fatalf("err = %v, want errSecretsEmpty", err)
	}
}

func TestSecretsParsePreservesUnknownLines(t *testing.T) {
	const in = "# Written by dot. Not tracked in git.\nset -gx bad 'lower'\nset -gx X 'un'quoted'\n\nset -x Y 'z'\nset -gx EXA_API_KEY 'v'\n"
	f := secretsParse(in)
	if got := string(f.render()); got != in {
		t.Errorf("render =\n%q\nwant\n%q", got, in)
	}
	if _, ok := f.get("X"); ok {
		t.Error("line with unescaped quote parsed as assignment")
	}
	if v, ok := f.get("EXA_API_KEY"); !ok || v != "v" {
		t.Errorf("EXA_API_KEY = %q, %v", v, ok)
	}
}
