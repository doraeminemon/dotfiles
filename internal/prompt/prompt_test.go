package prompt

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func accessible(input string) (*HuhPrompter, *bytes.Buffer) {
	var out bytes.Buffer
	return &HuhPrompter{In: strings.NewReader(input), Out: &out, Accessible: true}, &out
}

func TestHuhConfirmAccessible(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   bool
		want  bool
	}{
		{"yes", "y\n", false, true},
		{"no", "n\n", true, false},
		{"empty takes default", "\n", true, true},
		{"invalid then yes", "maybe\nyes\n", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, out := accessible(tt.input)
			got, err := p.Confirm("Continue?", tt.def)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if !strings.Contains(out.String(), "Continue?") {
				t.Errorf("title not printed: %q", out.String())
			}
		})
	}
}

func TestHuhSequentialPromptsShareReader(t *testing.T) {
	p, _ := accessible("y\nalice\n")
	ok, err := p.Confirm("Go?", false)
	if err != nil || !ok {
		t.Fatalf("Confirm = %v, %v", ok, err)
	}
	name, err := p.Input("Name", InputOpts{})
	if err != nil || name != "alice" {
		t.Fatalf("Input = %q, %v", name, err)
	}
}

func TestHuhInputAccessible(t *testing.T) {
	notEmpty := func(s string) error {
		if s == "" {
			return errors.New("required")
		}
		return nil
	}
	p, out := accessible("\nbob\n")
	got, err := p.Input("Name", InputOpts{Validate: notEmpty})
	if err != nil {
		t.Fatal(err)
	}
	if got != "bob" {
		t.Errorf("got %q, want bob", got)
	}
	if !strings.Contains(out.String(), "required") {
		t.Errorf("validation message not printed: %q", out.String())
	}
}

func TestHuhInputAccessibleInvalidAtEOF(t *testing.T) {
	p, _ := accessible("")
	_, err := p.Input("Name", InputOpts{Validate: func(string) error { return errors.New("required") }})
	var ie *InvalidInputError
	if !errors.As(err, &ie) || ie.Title != "Name" {
		t.Fatalf("err = %v, want *InvalidInputError", err)
	}
}

func TestHuhMultiSelectAccessible(t *testing.T) {
	opts := []Option{
		{Label: "Alpha", Value: "a"},
		{Label: "Beta", Value: "b", Selected: true},
		{Label: "Gamma", Value: "c"},
	}
	// Toggle Alpha on, Beta off, Gamma on, then finish with 0.
	p, out := accessible("1\n2\n3\n0\n")
	got, err := p.MultiSelect("Pick", opts)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("got %v, want [a c]", got)
	}
	if !strings.Contains(out.String(), "Gamma") {
		t.Errorf("options not printed: %q", out.String())
	}
}

func TestHuhSecretWithoutTTY(t *testing.T) {
	p, _ := accessible("hunter2\n")
	_, err := p.Input("API key", InputOpts{Secret: true})
	var se *SecretNeedsTTYError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *SecretNeedsTTYError", err)
	}
	if se.Title != "API key" {
		t.Errorf("Title = %q", se.Title)
	}
}

func TestHuhSpinnerAccessible(t *testing.T) {
	p, out := accessible("")
	want := errors.New("boom")
	err := p.Spinner(context.Background(), "Working", func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if !strings.Contains(out.String(), "Working") {
		t.Errorf("title not printed: %q", out.String())
	}
}

func TestFakePrompter(t *testing.T) {
	f := &FakePrompter{
		Confirms:   []bool{true},
		Inputs:     []string{"x"},
		Selections: [][]string{{"a", "b"}},
	}
	if v, err := f.Confirm("c1", false); err != nil || !v {
		t.Fatalf("Confirm = %v, %v", v, err)
	}
	if v, err := f.Input("i1", InputOpts{}); err != nil || v != "x" {
		t.Fatalf("Input = %q, %v", v, err)
	}
	if v, err := f.MultiSelect("m1", nil); err != nil || !slices.Equal(v, []string{"a", "b"}) {
		t.Fatalf("MultiSelect = %v, %v", v, err)
	}

	for _, tc := range []struct {
		kind Kind
		call func() error
	}{
		{KindConfirm, func() error { _, err := f.Confirm("c2", false); return err }},
		{KindInput, func() error { _, err := f.Input("i2", InputOpts{}); return err }},
		{KindMultiSelect, func() error { _, err := f.MultiSelect("m2", nil); return err }},
	} {
		var se *ScriptExhaustedError
		if err := tc.call(); !errors.As(err, &se) || se.Kind != tc.kind {
			t.Errorf("%s: err = %v, want ScriptExhaustedError", tc.kind, err)
		}
	}

	want := []string{"c1", "i1", "m1", "c2", "i2", "m2"}
	if !slices.Equal(f.Asked, want) {
		t.Errorf("Asked = %v, want %v", f.Asked, want)
	}
}

func TestFakePrompterRunsValidate(t *testing.T) {
	bad := errors.New("bad")
	f := &FakePrompter{Inputs: []string{"nope"}}
	_, err := f.Input("i", InputOpts{Validate: func(string) error { return bad }})
	if !errors.Is(err, bad) {
		t.Fatalf("err = %v, want %v", err, bad)
	}
}
