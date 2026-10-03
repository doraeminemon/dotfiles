package runner

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCmdString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Cmd
		want string
	}{
		{"plain", Cmd{Name: "brew", Args: []string{"install", "git"}}, "brew install git"},
		{"spaces", Cmd{Name: "sh", Args: []string{"-c", "exit 3"}}, "sh -c 'exit 3'"},
		{"single quote", Cmd{Name: "echo", Args: []string{"it's"}}, `echo 'it'\''s'`},
		{"empty arg", Cmd{Name: "git", Args: []string{"commit", "-m", ""}}, "git commit -m ''"},
		{"env", Cmd{Name: "make", Env: []string{"CC=clang", "FLAGS=-O2 -g"}}, "CC=clang FLAGS='-O2 -g' make"},
		{"safe punctuation", Cmd{Name: "git", Args: []string{"clone", "https://github.com/a/b.git", "dir_x@1+2,3"}}, "git clone https://github.com/a/b.git dir_x@1+2,3"},
		{"shell metachars", Cmd{Name: "echo", Args: []string{"$HOME", "a;b", "*"}}, "echo '$HOME' 'a;b' '*'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cmd.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecRunnerExitCode(t *testing.T) {
	r := &ExecRunner{}
	err := r.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "exit 3"}})
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if ce.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", ce.ExitCode)
	}
	if got, want := ce.Cmd.String(), "sh -c 'exit 3'"; got != want {
		t.Errorf("Cmd = %q, want %q", got, want)
	}
}

func TestExecRunnerStderrCaptured(t *testing.T) {
	r := &ExecRunner{}
	_, err := r.Output(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "echo err >&2; exit 1"}})
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if ce.ExitCode != 1 || ce.Stderr != "err\n" {
		t.Errorf("got ExitCode=%d Stderr=%q, want 1 %q", ce.ExitCode, ce.Stderr, "err\n")
	}
	if !strings.HasSuffix(ce.Error(), "exit status 1: err") {
		t.Errorf("Error() = %q", ce.Error())
	}
}

func TestExecRunnerStderrCapped(t *testing.T) {
	r := &ExecRunner{}
	script := "head -c 20000 /dev/zero | tr '\\0' x >&2; printf END >&2; exit 1"
	err := r.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", script}})
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if len(ce.Stderr) != StderrLimit {
		t.Errorf("len(Stderr) = %d, want %d", len(ce.Stderr), StderrLimit)
	}
	if !strings.HasSuffix(ce.Stderr, "END") {
		t.Errorf("Stderr does not keep the tail")
	}
}

func TestExecRunnerNotFound(t *testing.T) {
	r := &ExecRunner{}
	err := r.Run(context.Background(), Cmd{Name: "dot-no-such-binary-xyz"})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want *NotFoundError", err)
	}
	if nf.Name != "dot-no-such-binary-xyz" {
		t.Errorf("Name = %q", nf.Name)
	}
}

func TestExecRunnerOutput(t *testing.T) {
	r := &ExecRunner{}
	dir := t.TempDir()
	out, err := r.Output(context.Background(), Cmd{
		Name:  "sh",
		Args:  []string{"-c", `printf '%s|%s|' "$DOT_TEST_VAR" "$(pwd -P)"; cat; echo noise >&2`},
		Env:   []string{"DOT_TEST_VAR=hello"},
		Dir:   dir,
		Stdin: strings.NewReader("in"),
	})
	if err != nil {
		t.Fatal(err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "hello|"+realDir+"|in"; got != want {
		t.Errorf("Output = %q, want %q", got, want)
	}
}

func TestExecRunnerLog(t *testing.T) {
	var log bytes.Buffer
	r := &ExecRunner{Log: &log}
	if err := r.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "echo out; echo warn >&2"}}); err != nil {
		t.Fatal(err)
	}
	if got := log.String(); !strings.Contains(got, "out\n") || !strings.Contains(got, "warn\n") {
		t.Errorf("Log = %q", got)
	}
}

func TestExecRunnerContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&ExecRunner{}).Run(ctx, Cmd{Name: "sleep", Args: []string{"5"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDryRunner(t *testing.T) {
	var out bytes.Buffer
	d := &DryRunner{Out: &out}
	ctx := context.Background()
	if err := d.Run(ctx, Cmd{Name: "brew", Args: []string{"install", "git"}}); err != nil {
		t.Fatal(err)
	}
	b, err := d.Output(ctx, Cmd{Name: "sh", Args: []string{"-c", "exit 3"}})
	if err != nil || b != nil {
		t.Fatalf("Output = %q, %v; want nil, nil", b, err)
	}
	want := "+ brew install git\n+ sh -c 'exit 3'\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

func TestFakeRunner(t *testing.T) {
	boom := errors.New("boom")
	f := &FakeRunner{Script: map[string]Result{
		"git rev-parse HEAD": {Stdout: []byte("abc\n")},
		"git":                {Err: boom},
	}}
	ctx := context.Background()

	out, err := f.Output(ctx, Cmd{Name: "git", Args: []string{"rev-parse", "HEAD"}})
	if err != nil || string(out) != "abc\n" {
		t.Errorf("exact key: got %q, %v", out, err)
	}
	if err := f.Run(ctx, Cmd{Name: "git", Args: []string{"pull"}}); !errors.Is(err, boom) {
		t.Errorf("name fallback: err = %v, want boom", err)
	}
	if err := f.Run(ctx, Cmd{Name: "brew", Args: []string{"update"}}); err != nil {
		t.Errorf("unscripted: err = %v, want nil", err)
	}

	want := []string{"git rev-parse HEAD", "git pull", "brew update"}
	if got := f.Commands(); !slices.Equal(got, want) {
		t.Errorf("Commands() = %q, want %q", got, want)
	}
	if len(f.Calls) != 3 || f.Calls[2].Name != "brew" {
		t.Errorf("Calls = %+v", f.Calls)
	}
}
