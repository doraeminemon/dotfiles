# 0001. Go CLI installer shipped as static binaries

## Status
Accepted, 2026-10-03.

## Context
The repo must set up a new macOS or Linux machine fast. The installer must be typed and testable. It must run on a machine that has no git, Go, or Homebrew yet. chezmoi symlinks need the repo checked out on disk.

## Decision
The installer is a Go CLI named `dot`, with prompts from `charm.land/huh/v2`. goreleaser builds static binaries (`CGO_ENABLED=0`, darwin and linux, amd64 and arm64) and publishes them on GitHub Releases. Asset names match `uname -s` and `uname -m`, so one `curl` line fetches the right binary.

The binary does not embed config. The `repo` module clones this repo over HTTPS, and every other module reads packages, `home/`, and skills from `RepoDir`. `go:embed` cannot replace the clone, because chezmoi symlinks must point into a real checkout.

## Consequences
- Modules use typed interfaces (`Runner`, `Prompter`) and run in tests with fakes on both platforms.
- A new machine needs only `curl`. No bootstrap script exists.
- A change to `dot` itself needs a `v*` tag before a new machine gets it. A config change does not. On a machine with the checkout, use `go run ./cmd/dot`.
- We own a release pipeline (goreleaser, GitHub Actions).

## Alternatives considered
- **bash + gum**: weak typing, no practical tests, and fragile quoting. Lost on correctness.
- **Nix / home-manager**: steep learning curve, and it replaces the plain brew and apt lists that we want to keep. Lost on cost for one user.
- **Ansible**: heavy for one user and needs Python on the new machine. Lost on weight.
- **`bootstrap.sh` that installs brew and Go, then runs `go run`**: needs a package manager and a toolchain before anything runs, and it is still a shell script. Lost on bootstrap cost.
- **`go:embed` for config instead of a clone**: chezmoi symlink mode needs a checkout on disk, so the clone is required anyway. Lost on duplication.
