// Package platform detects the operating system, CPU architecture, and
// system package manager of the current machine.
//
// Detection is a pure function of its inputs (GOOS, GOARCH, and a PATH
// lookup), so modules and tests can build any supported Platform value
// without touching the host. Current wraps Detect for the real machine and
// belongs at the program edge.
package platform

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OS is a supported operating system.
type OS int

// Supported operating systems. The zero value is not a valid OS, so an
// unset Platform never passes for a real one.
const (
	Darwin OS = iota + 1
	Linux
)

// String returns the GOOS spelling of o, for example "darwin".
func (o OS) String() string {
	switch o {
	case Darwin:
		return "darwin"
	case Linux:
		return "linux"
	}
	return fmt.Sprintf("OS(%d)", int(o))
}

// ParseOS parses a GOOS value. It returns UnsupportedOSError for any OS
// other than darwin and linux.
func ParseOS(goos string) (OS, error) {
	switch goos {
	case "darwin":
		return Darwin, nil
	case "linux":
		return Linux, nil
	default:
		return 0, UnsupportedOSError{GOOS: goos}
	}
}

// Arch is a supported CPU architecture.
type Arch int

// Supported CPU architectures. The zero value is not a valid Arch.
const (
	AMD64 Arch = iota + 1
	ARM64
)

// String returns the GOARCH spelling of a, for example "amd64".
func (a Arch) String() string {
	switch a {
	case AMD64:
		return "amd64"
	case ARM64:
		return "arm64"
	}
	return fmt.Sprintf("Arch(%d)", int(a))
}

// ParseArch parses a GOARCH value. It returns UnsupportedArchError for any
// architecture other than amd64 and arm64.
func ParseArch(goarch string) (Arch, error) {
	switch goarch {
	case "amd64":
		return AMD64, nil
	case "arm64":
		return ARM64, nil
	default:
		return 0, UnsupportedArchError{GOARCH: goarch}
	}
}

// PkgMgr is the system package manager that the installer can drive.
type PkgMgr int

// Package managers. NoPkgMgr means the installer drives no system package
// manager (macOS, or a Linux distribution without apt).
const (
	NoPkgMgr PkgMgr = iota
	Apt
)

// String returns a lowercase name for m, for example "apt".
func (m PkgMgr) String() string {
	switch m {
	case NoPkgMgr:
		return "none"
	case Apt:
		return "apt"
	}
	return fmt.Sprintf("PkgMgr(%d)", int(m))
}

// Platform describes the machine that the installer runs on.
type Platform struct {
	OS     OS
	Arch   Arch
	PkgMgr PkgMgr
}

// String returns "os/arch", with the package manager appended when there is
// one, for example "linux/arm64 (apt)".
func (p Platform) String() string {
	s := p.OS.String() + "/" + p.Arch.String()
	if p.PkgMgr != NoPkgMgr {
		s += " (" + p.PkgMgr.String() + ")"
	}
	return s
}

// UnameMachine returns what `uname -m` prints on p: "x86_64" for amd64, and
// "arm64" on darwin or "aarch64" on linux for arm64. Release assets and
// third-party installer downloads use this spelling.
func (p Platform) UnameMachine() string {
	switch p.Arch {
	case AMD64:
		return "x86_64"
	case ARM64:
		if p.OS == Linux {
			return "aarch64"
		}
		return "arm64"
	}
	return p.Arch.String()
}

// aptBinary is the executable whose presence on PATH selects Apt.
const aptBinary = "apt-get"

// Detect builds a Platform from GOOS and GOARCH values and a PATH lookup.
// lookPath has the signature of exec.LookPath; Detect calls it only on
// linux, to find apt-get. Detect returns UnsupportedOSError or
// UnsupportedArchError when goos or goarch is not supported.
func Detect(goos, goarch string, lookPath func(string) (string, error)) (Platform, error) {
	o, err := ParseOS(goos)
	if err != nil {
		return Platform{}, err
	}
	a, err := ParseArch(goarch)
	if err != nil {
		return Platform{}, err
	}
	pm := NoPkgMgr
	if o == Linux {
		if _, err := lookPath(aptBinary); err == nil {
			pm = Apt
		}
	}
	return Platform{OS: o, Arch: a, PkgMgr: pm}, nil
}

// Current detects the platform of the running process from runtime.GOOS,
// runtime.GOARCH, and exec.LookPath.
func Current() (Platform, error) {
	return Detect(runtime.GOOS, runtime.GOARCH, exec.LookPath)
}

// UnsupportedOSError reports a GOOS that the installer does not support.
type UnsupportedOSError struct {
	GOOS string
}

// Error implements error.
func (e UnsupportedOSError) Error() string {
	return fmt.Sprintf("unsupported operating system %q: want darwin or linux", e.GOOS)
}

// UnsupportedArchError reports a GOARCH that the installer does not support.
type UnsupportedArchError struct {
	GOARCH string
}

// Error implements error.
func (e UnsupportedArchError) Error() string {
	return fmt.Sprintf("unsupported architecture %q: want amd64 or arm64", e.GOARCH)
}
