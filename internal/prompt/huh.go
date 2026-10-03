package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"golang.org/x/term"
)

var _ Prompter = (*HuhPrompter)(nil)

// HuhPrompter asks questions with charm.land/huh/v2.
//
// A nil In or Out uses huh's default (stdin; stderr, or stdout in accessible
// mode). Accessible switches huh to plain numbered prompts on In/Out, which is
// the fallback when stdin is not a terminal.
type HuhPrompter struct {
	In         io.Reader
	Out        io.Writer
	Accessible bool
}

// NewHuhPrompter returns a HuhPrompter on stdin and stderr. It turns on
// accessible mode when stdin is not a terminal or ACCESSIBLE is set.
func NewHuhPrompter() *HuhPrompter {
	return &HuhPrompter{
		In:         os.Stdin,
		Out:        os.Stderr,
		Accessible: os.Getenv("ACCESSIBLE") != "" || !term.IsTerminal(int(os.Stdin.Fd())),
	}
}

// Confirm implements Prompter.
func (p *HuhPrompter) Confirm(title string, def bool) (bool, error) {
	v := def
	f := huh.NewConfirm().Title(title).Value(&v)
	if err := p.run(title, p.in(), f); err != nil {
		return false, err
	}
	return v, nil
}

// Input implements Prompter.
func (p *HuhPrompter) Input(title string, opts InputOpts) (string, error) {
	var v string
	f := huh.NewInput().Title(title).Placeholder(opts.Placeholder).Value(&v)
	if opts.Validate != nil {
		f = f.Validate(opts.Validate)
	}
	in := p.in()
	if opts.Secret {
		f = f.EchoMode(huh.EchoModePassword)
		if p.Accessible {
			// huh reads a password from the reader's file descriptor, and in
			// accessible mode it drops the error when there is none. Check first.
			if !isTerminal(p.In) {
				return "", &SecretNeedsTTYError{Title: title}
			}
			in = p.In
		}
	}
	if err := p.run(title, in, f); err != nil {
		return "", err
	}
	// Accessible mode returns the last answer when input ends, valid or not.
	if p.Accessible && opts.Validate != nil {
		if err := opts.Validate(v); err != nil {
			return "", &InvalidInputError{Title: title, Err: err}
		}
	}
	return v, nil
}

// MultiSelect implements Prompter.
func (p *HuhPrompter) MultiSelect(title string, opts []Option) ([]string, error) {
	hopts := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		hopts[i] = huh.NewOption(o.Label, o.Value).Selected(o.Selected)
	}
	var v []string
	f := huh.NewMultiSelect[string]().Title(title).Options(hopts...).Value(&v)
	if err := p.run(title, p.in(), f); err != nil {
		return nil, err
	}
	return v, nil
}

// Spinner shows title while fn runs and returns fn's error. It is a method on
// HuhPrompter, not part of Prompter, so it shares the Accessible flag and Out;
// in accessible mode it prints title once instead of animating.
func (p *HuhPrompter) Spinner(ctx context.Context, title string, fn func(context.Context) error) error {
	s := spinner.New().Title(title).Context(ctx).ActionWithErr(fn).WithAccessible(p.Accessible)
	if p.Out != nil {
		s = s.WithOutput(p.Out)
	}
	if p.In != nil {
		s = s.WithInput(p.In)
	}
	return s.Run()
}

func (p *HuhPrompter) run(title string, in io.Reader, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(p.Accessible).WithShowHelp(!p.Accessible)
	if in != nil {
		form = form.WithInput(in)
	}
	if p.Out != nil {
		form = form.WithOutput(p.Out)
	}
	err := form.Run()
	switch {
	case err == nil:
		return nil
	case errors.Is(err, huh.ErrUserAborted):
		return ErrAborted
	default:
		return fmt.Errorf("prompt %q: %w", title, err)
	}
}

// in returns the reader for non-secret fields. In accessible mode huh builds a
// new bufio.Scanner per prompt, which reads ahead and loses the lines meant for
// later prompts on the same stream. Reading one byte at a time stops each
// scanner at its own newline.
func (p *HuhPrompter) in() io.Reader {
	if p.In == nil || !p.Accessible {
		return p.In
	}
	return byteReader{p.In}
}

type byteReader struct{ r io.Reader }

func (b byteReader) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	return b.r.Read(buf[:1])
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}
