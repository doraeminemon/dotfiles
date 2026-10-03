package platform

import (
	"errors"
	"os/exec"
	"testing"
)

func lookPathFound(name string) (string, error) { return "/usr/bin/" + name, nil }

func lookPathMissing(string) (string, error) { return "", exec.ErrNotFound }

func lookPathPanics(string) (string, error) {
	panic("lookPath must not be called")
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		goarch   string
		lookPath func(string) (string, error)
		want     Platform
		wantOS   *UnsupportedOSError
		wantArch *UnsupportedArchError
	}{
		{
			name: "darwin arm64 ignores PATH", goos: "darwin", goarch: "arm64", lookPath: lookPathPanics,
			want: Platform{OS: Darwin, Arch: ARM64, PkgMgr: NoPkgMgr},
		},
		{
			name: "darwin amd64", goos: "darwin", goarch: "amd64", lookPath: lookPathPanics,
			want: Platform{OS: Darwin, Arch: AMD64, PkgMgr: NoPkgMgr},
		},
		{
			name: "linux with apt", goos: "linux", goarch: "amd64", lookPath: lookPathFound,
			want: Platform{OS: Linux, Arch: AMD64, PkgMgr: Apt},
		},
		{
			name: "linux without apt", goos: "linux", goarch: "arm64", lookPath: lookPathMissing,
			want: Platform{OS: Linux, Arch: ARM64, PkgMgr: NoPkgMgr},
		},
		{
			name: "windows", goos: "windows", goarch: "amd64", lookPath: lookPathPanics,
			wantOS: &UnsupportedOSError{GOOS: "windows"},
		},
		{
			name: "unsupported arch", goos: "linux", goarch: "386", lookPath: lookPathPanics,
			wantArch: &UnsupportedArchError{GOARCH: "386"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(tt.goos, tt.goarch, tt.lookPath)
			switch {
			case tt.wantOS != nil:
				var e UnsupportedOSError
				if !errors.As(err, &e) || e != *tt.wantOS {
					t.Fatalf("Detect() error = %v, want %v", err, *tt.wantOS)
				}
			case tt.wantArch != nil:
				var e UnsupportedArchError
				if !errors.As(err, &e) || e != *tt.wantArch {
					t.Fatalf("Detect() error = %v, want %v", err, *tt.wantArch)
				}
			default:
				if err != nil {
					t.Fatalf("Detect() error = %v", err)
				}
				if got != tt.want {
					t.Fatalf("Detect() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestUnameMachine(t *testing.T) {
	tests := []struct {
		p    Platform
		want string
	}{
		{Platform{OS: Darwin, Arch: AMD64}, "x86_64"},
		{Platform{OS: Darwin, Arch: ARM64}, "arm64"},
		{Platform{OS: Linux, Arch: AMD64}, "x86_64"},
		{Platform{OS: Linux, Arch: ARM64}, "aarch64"},
	}
	for _, tt := range tests {
		t.Run(tt.p.String(), func(t *testing.T) {
			if got := tt.p.UnameMachine(); got != tt.want {
				t.Fatalf("UnameMachine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{Darwin.String(), "darwin"},
		{Linux.String(), "linux"},
		{OS(9).String(), "OS(9)"},
		{AMD64.String(), "amd64"},
		{ARM64.String(), "arm64"},
		{Arch(9).String(), "Arch(9)"},
		{NoPkgMgr.String(), "none"},
		{Apt.String(), "apt"},
		{PkgMgr(9).String(), "PkgMgr(9)"},
		{Platform{OS: Darwin, Arch: ARM64}.String(), "darwin/arm64"},
		{Platform{OS: Linux, Arch: AMD64, PkgMgr: Apt}.String(), "linux/amd64 (apt)"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("String() = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, o := range []OS{Darwin, Linux} {
		if got, err := ParseOS(o.String()); err != nil || got != o {
			t.Errorf("ParseOS(%q) = %v, %v", o, got, err)
		}
	}
	for _, a := range []Arch{AMD64, ARM64} {
		if got, err := ParseArch(a.String()); err != nil || got != a {
			t.Errorf("ParseArch(%q) = %v, %v", a, got, err)
		}
	}
}

func TestCurrent(t *testing.T) {
	if _, err := Current(); err != nil {
		t.Fatalf("Current() error = %v", err)
	}
}

func TestZeroPlatformIsInvalid(t *testing.T) {
	var p Platform
	if p.OS == Darwin || p.OS == Linux {
		t.Fatalf("zero OS = %v, want no valid OS", p.OS)
	}
	if p.Arch == AMD64 || p.Arch == ARM64 {
		t.Fatalf("zero Arch = %v, want no valid Arch", p.Arch)
	}
}
