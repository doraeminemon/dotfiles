# Plan: dotfiles as a Go project (`dot` CLI + chezmoi)

## Context
Goal: one public repo (`github.com/doraeminemon/dotfiles`) that sets up a new macOS or Linux machine fast. The repo is a **Go module**. The installer is a Go CLI named `dot` with an interactive TUI (charmbracelet/huh, the library behind gum). When stdin is not a TTY, huh's built-in accessible mode gives plain numbered prompts instead. Config files in `$HOME` are managed by chezmoi in **symlink mode**, with this repo as the source. Secrets and personal info are not in the repo. The CLI asks for them at install time.

Answers you gave: exa = **Exa API key**. Installer = interactive with fallback (now built in Go, not bash+gum). chezmoi = **$HOME configs only, symlink mode**. Repo = **doraeminemon/dotfiles**, public.

## What the survey of this machine found
- **Shell**: fish + fisher plugins (`patrickf1/fzf.fish`, `franciscolourenco/done`, `kidonng/zoxide.fish`, `jamiesteven/fish-plugin-lsd`, `edc/bass`), starship (`~/.config/starship.toml`, nerd-font symbols), mise activation in `config.fish`.
- **mise** (`~/.config/mise/config.toml`): bun, go, node 22 (musl), pnpm 10, python, rust.
- **brew leaves**: many tools. Core subset kept below. Environment-specific tools (flyctl, koyeb, neonctl, infisical, snyk, elixir, caddy, …) are left out and listed in README as "install by hand".
- **Claude**: `~/.claude/{CLAUDE.md, RTK.md, rules/context7.md, settings.json, agents/, hooks/}`. `settings.json` has a hard-coded absolute home path and a personal `autoMode` block. Both get scrubbed.
- **Skills**: global lock `~/.agents/.skill-lock.json` (sources: doraeminemon/agent-skills, firecrawl/*, multica-ai/andrej-karpathy-skills, vercel-labs/skills). Personal skills live in the `doraeminemon/agent-skills` repo. Per-project `skills-lock.json` files are in challenge-ai, claude-go-statusline, personal-cv, personal-finance-agent, personal-playground-website, wedding-invitation, and zdb.
- **Project stacks**: SvelteKit + Cloudflare (pnpm/bun, wrangler), Python with uv (two projects), Rust, Go, Typst.
- **AGENTS.md / CLAUDE.md variants**: several personal projects carry their own.
- **Sensitive values to scrub**: gitconfig email, the Snyk org env var, `~/.npmrc` tokens (excluded entirely), absolute home paths, ssh private keys (never copied), Fig/OrbStack/CodeSandbox lines (made conditional or dropped).

## Repository layout
```
go.mod                      module github.com/doraeminemon/dotfiles (go 1.25)
cmd/dot/main.go             CLI entry. Parses args, builds Env, runs at the edge
internal/
  platform/                 Platform{OS: Darwin|Linux, Arch, PkgMgr: Apt|None}, Detect()
  runner/                   Runner interface; ExecRunner, DryRunner (prints), FakeRunner (tests)
  prompt/                   Prompter interface (consumer-owned); one adapter over huh v2. No hand-rolled fallback
  pkglist/                  ParseList(io.Reader) ([]Package, error) for packages/*.txt
  module/                   Module interface, ID type, Status, registry, ordered Plan
  modules/                  one file per module (see table)
packages/                   brew.txt, brew-cask-darwin.txt, apt.txt (one pkg per line, # comments)
home/                       chezmoi source root (repo has .chezmoiroot = "home")
  .chezmoi.toml.tmpl        promptStringOnce name/email; [chezmoi] mode = "symlink"
  .chezmoiignore            OS-conditional entries
  dot_config/fish/{config.fish, fish_plugins, conf.d/*.fish}
  dot_config/starship.toml
  dot_config/mise/config.toml
  dot_config/git/ignore
  dot_gitconfig.tmpl        name/email from prompt data (template, so copied, not symlinked)
  private_dot_ssh/config.tmpl  github.com block; UseKeychain only on darwin
  dot_claude/{CLAUDE.md, RTK.md, rules/, agents/, executable_hooks/}
  dot_claude/settings.json.tmpl  $HOME paths, no autoMode block
skills/
  global.skill-lock.json    copy of ~/.agents/.skill-lock.json
  profiles/*.json           skills-lock presets: sveltekit-cloudflare, typescript-strict, rust, python-uv, neon, go
agents/                     AGENTS.md/CLAUDE.md templates: base, typescript-sveltekit, rust, python, go
docs/adr/0001..0004
AGENTS.md, README.md, justfile, .golangci.yml, .goreleaser.yaml, .github/workflows/{ci,release}.yml
```

## Types for review (workflow steps 2–5)
```go
// internal/module
type ID string                      // parsed from a fixed registry; unknown --only values are an error
type Status int                     // StatusMissing | StatusInstalled | StatusUnsupported
type Env struct {
    Platform platform.Platform
    Run      runner.Runner          // all exec goes through this (inverted for tests/dry-run)
    Ask      prompt.Prompter        // all user input goes through this
    FS       fs.FS / small WriteFS interface (consumer-owned: WriteFile, MkdirAll, Stat)
    RepoDir  string
    Home     string
}
type Module interface {
    ID() ID
    Summary() string
    DependsOn() []ID                // e.g. fish → brew; skills → mise(bun/node)
    Supports(platform.Platform) bool
    Check(ctx context.Context, env Env) (Status, error)
    Apply(ctx context.Context, env Env) error
}

// internal/runner
type Cmd struct{ Name string; Args []string; Env []string; Stdin io.Reader }
type Runner interface{ Run(ctx context.Context, c Cmd) error; Output(ctx context.Context, c Cmd) ([]byte, error) }

// internal/prompt
type Prompter interface {
    Confirm(title string, def bool) (bool, error)
    Input(title string, opt InputOpts) (string, error)   // InputOpts{Secret bool; Validate func(string) error}
    MultiSelect(title string, opts []Option) ([]string, error)
}

// typed errors (errors.As at the CLI edge)
type UnsupportedPlatformError struct{ Module ID; Platform platform.Platform }
type CommandError struct{ Cmd Cmd; ExitCode int; Stderr string }
type UnknownModuleError struct{ Name string }
```
Brand-like safety comes from distinct named types (`ID`, `ProfileName`). Each is built only by a parse function that checks against the registry or the `skills/profiles` dir.

## Modules (run in dependency order)
| ID | macOS | Linux |
|---|---|---|
| `repo` | ensure Xcode CLT (`xcode-select --install`); clone repo over HTTPS to `~/Projects/dotfiles` if missing | git from `apt`; same clone |
| `apt` | skip | `apt-get install` from `packages/apt.txt` (build-essential, curl, git, file, procps, unzip, ca-certificates, fish) |
| `brew` | install Homebrew if missing; `brew install` from `brew.txt` + `brew-cask-darwin.txt` | Linuxbrew; `brew.txt` only |
| `chezmoi` | `chezmoi init --source $REPO`, show `chezmoi diff`, confirm, `chezmoi apply`. Back up replaced files to `~/.dotfiles-backup/<ts>` first | same |
| `fish` | add fish to `/etc/shells`, `chsh` (confirm), `fisher update` | same |
| `mise` | `mise install` (bun, go, node 22, pnpm 10, rust) | same |
| `python` | miniforge cask → conda; `uv tool install ruff ty` | miniforge installer script; same uv tools |
| `claude` | cask `claude-code` | official native install script |
| `ssh` | if no `~/.ssh/id_ed25519`: prompt email, `ssh-keygen -t ed25519`, add to agent (`--apple-use-keychain`), offer `gh auth login` + `gh ssh-key add` | same, no keychain |
| `secrets` | prompt `EXA_API_KEY` (masked). Write `~/.config/fish/conf.d/secrets.fish` with mode 0600. Not managed by chezmoi; gitignored pattern | same |
| `skills` | restore global skills from `skills/global.skill-lock.json` through the `skills` CLI (`npx skills add …`). Check exact flags with `--help` / ctx7 during implementation | same |

Core `brew.txt`: git, git-lfs, fish, fisher, starship, chezmoi, mise, gh, uv, jq, ripgrep, fd, fzf, zoxide, lsd, bat, just, ast-grep, glow, rtk, golangci-lint. Cask (darwin): claude-code, miniforge. pnpm, bun, and go come from mise, not brew. Python comes from conda (miniforge), so `python` is removed from mise config.

## CLI surface
- `dot install [--only fish,ssh] [--dry-run] [--yes]`: with no `--only`, a TUI multiselect of the modules that this platform supports. Each module shows Check status first.
- `dot list`: modules, their support on this platform, and their status.
- `dot doctor`: runs Check on all modules. Does not change anything.
- `dot skills apply <profile> [dir]`: copies `skills/profiles/<profile>.json` to `<dir>/skills-lock.json` and runs the skills install.
- `dot agents apply <template> [dir]`: writes `AGENTS.md` and a `CLAUDE.md` with `@AGENTS.md` into the project.

Flag parsing uses stdlib `flag` with subcommands, no cobra (YAGNI).

## Charm libraries (checked with ctx7 and `go list -m`, 2026-10-03)
| Need | Library | Notes |
|---|---|---|
| Prompts (multiselect, confirm, masked input) | `charm.land/huh/v2` v2.0.3 | `EchoMode(huh.EchoModePassword)` for secrets. `WithAccessible(true)` switches to plain numbered stdin prompts. This **is** the non-TTY fallback, so no PlainPrompter is written |
| Progress for quiet steps | `charm.land/huh/v2/spinner` | `ActionWithErr(func(ctx) error)` + `Context(ctx)` |
| Styled output, step headers | `charm.land/lipgloss/v2` v2.0.6, `charm.land/log/v2` v2.0.1 | |
| Full-screen TUI | `charm.land/bubbletea/v2` v2.0.10 | Not used now. huh covers the forms. Add it only for a live dashboard later |
| Real PTY | `github.com/charmbracelet/x/xpty` v0.1.4 | Not used now. Interactive commands (`sudo`, `chsh`, `ssh-keygen`, `gh auth login`, Homebrew installer) run with inherited stdin/stdout/stderr after the form closes, so they already have the real terminal. Use xpty only if we later embed command output in a TUI pane |

Import paths are `charm.land/...` (v2), not `github.com/charmbracelet/...`. Accessible mode turns on when stdin is not a TTY (`golang.org/x/term`) or when `ACCESSIBLE=1` is set.

## Distribution (no bootstrap.sh)
- goreleaser builds `CGO_ENABLED=0` binaries for darwin/linux × amd64/arm64. Asset names match `uname -s`/`uname -m` (`dot_Darwin_arm64`, `dot_Linux_x86_64`, …).
- New machine: `curl -fsSL "https://github.com/doraeminemon/dotfiles/releases/latest/download/dot_$(uname -s)_$(uname -m)" -o dot && chmod +x dot && ./dot install`.
- The binary does not embed config. chezmoi symlinks need a real checkout, so the `repo` module clones it, and every other module reads from `RepoDir`.
- Cost: a change to `dot` itself needs a `v*` tag before a new machine gets it. A config change does not. On a machine that already has the checkout, use `go run ./cmd/dot`.

## Migrating this machine
1. Copy the current configs into `home/`, scrubbed (paths → `{{ .chezmoi.homeDir }}` or `$HOME`, email → template data, Snyk org var → secrets module).
2. Get `skills/profiles/*` from the existing project `skills-lock.json` files, grouped by stack. Get `agents/*` templates from the existing AGENTS.md files, made generic.
3. Run `go run ./cmd/dot install --only chezmoi` on this Mac. It backs up the files, then replaces them with symlinks into the repo.

## Publishing
`git init` is done. Then: `.gitignore` (secrets.fish, .env*, *.local.*, dist/), run `gitleaks detect` over the tree and history, and grep for the macOS username, the personal email domain, `_authToken`, and the Snyk env var name. Then `gh repo create doraeminemon/dotfiles --public --source . --push`. **I will show you the scan result and ask before the push**, because a push to a public repo cannot be undone.

## ADRs (`docs/adr/`)
1. 0001 Go CLI as installer, shipped as static binaries on GitHub Releases. Rejected: bash+gum (weak typing and no tests), Nix/home-manager (steep learning curve; apt/brew lists asked for), Ansible (heavy for one user), `bootstrap.sh` + `go run` (needs brew and Go before anything runs).
2. 0002 chezmoi symlink mode with `.chezmoiroot`. Templates (gitconfig, ssh config, claude settings) are copied, not symlinked. This is a known trade-off.
3. 0003 brew on Linux for tools; apt only for system prerequisites. One package list to keep up to date.
4. 0004 secrets never in the repo; prompted at install time and written mode 0600.

## AGENTS.md (repo root)
Covers the layout map, how to add a module (one file in `internal/modules`, register it, add a Fake-runner test), how to add a package or a skills profile, scrubbing rules, the verification commands, and a list of open TODOs (release binaries via goreleaser, more Linux distros).

## Verification
- `go test ./...`: each module tested with `FakeRunner` + fake Prompter, on both a darwin and a linux `Platform` (asserts the exact commands it would run).
- `golangci-lint run`, `go vet ./...`.
- `go run ./cmd/dot install --dry-run --yes` on this Mac: prints the plan and runs nothing.
- `chezmoi diff --source .` before apply. After apply, `ls -l ~/.config/fish/config.fish` shows a symlink into the repo, and a new fish shell starts clean.
- Linux: CI matrix (ubuntu-latest, macos-latest) runs tests, `dot install --dry-run --yes`, and `goreleaser check`. Optional manual test: `orb create ubuntu dottest` and the curl one-liner inside it (OrbStack is installed).
- Secret scan passes before the push.
