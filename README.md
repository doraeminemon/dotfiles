# dotfiles

Dotfiles and a machine setup tool for macOS and Linux. The tool is a Go CLI named `dot`. It installs packages, links config files into `$HOME`, and sets up the shell, language tools, SSH, and agent skills. Config files are managed by [chezmoi](https://www.chezmoi.io) in symlink mode, so an edit in `$HOME` is an edit in this repo. Secrets and personal data are not in the repo. `dot` asks for them at install time.

## New machine

```sh
curl -fsSL "https://github.com/doraeminemon/dotfiles/releases/latest/download/dot_$(uname -s)_$(uname -m)" -o dot && chmod +x dot && ./dot install
```

The binary holds no config. The `repo` module clones this repo to `~/Projects/dotfiles`. Every other module reads from that checkout. Run `./dot version` to see which build you have.

On a terminal, `dot install` shows a list of the modules for your platform, with their status. Pick the ones to run. Without a terminal, or with `--yes`, it runs all of them.

## This machine (already set up)

Run from the checkout:

```sh
go run ./cmd/dot install
```

`dot` uses the checkout that contains your current directory. A change to `dot` itself works at once with `go run`. A new machine gets it after the next `v*` release tag.

## What `dot install` does

Modules run in dependency order. For each module, `dot` runs Check first and skips the module if it is already installed. The order is:

`repo` → `apt` (Linux only) → `brew` → `chezmoi` → `secrets` → `fish` → `mise` → `python` → `claude` → `ssh` → `skills`

| ID | macOS | Linux | What it does |
|---|---|---|---|
| `repo` | Needs the Xcode Command Line Tools (asks you to install them) | git comes from `apt` | Clones this repo over HTTPS to `~/Projects/dotfiles` if it is not there |
| `apt` | Skipped | Debian and Ubuntu: `apt-get install` of `packages/apt.txt` | System prerequisites for Homebrew, and fish |
| `brew` | Installs Homebrew; formulae from `packages/brew.txt`; casks from `packages/brew-cask-darwin.txt` | Installs Homebrew (Linuxbrew); formulae only | Puts the core tools in place and on `PATH` for the next modules |
| `chezmoi` | Yes | Yes | `chezmoi init` with this repo as source, shows the diff, asks, backs up replaced files to `~/.dotfiles-backup/<timestamp>`, then applies as symlinks. Asks for your name and email, used by templates |
| `secrets` | Yes | Yes | Prompts for `EXA_API_KEY` and writes `~/.config/fish/conf.d/secrets.fish` |
| `fish` | Yes | Yes | Adds fish to `/etc/shells`, sets it as the login shell, runs `fisher update` |
| `mise` | Yes | Yes | `mise install` for the global mise config (bun, go, node, pnpm, rust) |
| `python` | miniforge cask (from `brew`) | Downloads the Miniforge installer to `~/miniforge3` | Runs `conda init fish`, turns off `auto_activate_base`, installs the uv tools `ruff` and `ty` |
| `claude` | Cask `claude-code` (from `brew`) | Official native install script | Installs Claude Code |
| `ssh` | Yes | Yes | Creates an ed25519 key if missing, adds it to the agent, uploads the public key to GitHub with `gh` |
| `skills` | Yes | Yes | Restores the global agent skills from `skills/global.skill-lock.json` with the `skills` CLI |

## Commands

Flags go before positional arguments.

```
dot install [--only a,b] [--dry-run] [--yes] [--repo DIR]
dot list [--repo DIR]
dot doctor [--repo DIR]
dot skills apply [--force] [--repo DIR] <profile> [dir]
dot agents apply [--force] [--repo DIR] <template> [dir]
dot version
dot help
```

- `install`: run the modules. `--only fish,ssh` runs those modules and their dependencies. `--yes` answers yes to every confirmation and skips the module picker. `--repo` sets the checkout to read from.
- `list`: show each module, whether this platform supports it, and its status.
- `doctor`: run Check on every module and report. It changes nothing.
- `skills apply <profile> [dir]`: copy `skills/profiles/<profile>.json` to `<dir>/skills-lock.json` (default: the current directory) and restore the skills. `--force` overwrites an existing file.
- `agents apply <template> [dir]`: write `agents/<template>.md` to `<dir>/AGENTS.md`, and `agents/CLAUDE.md` to `<dir>/CLAUDE.md`. `--force` overwrites.
- `version`: print the build version.

### Dry run

`dot install --dry-run --yes` shows what would happen and changes nothing.

- A line that starts with `+` is a command that changes the system. It is printed and not run.
- A line that starts with `?` is a read-only query. It is run, so Check sees the real state of the machine.
- File writes, mkdir, moves, and removals are printed as `+` lines and not done.
- A dry run continues past a module error. It prints `! <id>: <error>`, goes on with the next module, then prints a summary of the modules that failed and exits 1. A real install stops at the first error.

Commands that need the terminal (sudo, chsh, ssh-keygen, gh auth) are printed and never run in a dry run.

## Secrets

The `secrets` module asks for `EXA_API_KEY` with a masked prompt. It writes `~/.config/fish/conf.d/secrets.fish` with mode 0600. The file is not in the repo and chezmoi does not manage it.

Without a terminal, set `DOT_EXA_API_KEY` in the environment. The module reads that instead. If neither is available, it fails.

To add a secret, add an entry to `secretsList` in `internal/modules/secrets.go`.

## Machine-only config

Some config belongs to one machine and stays out of the repo.

- `~/.ssh/config.local`: the managed `~/.ssh/config` includes it on its first line. Put private hosts there. ssh ignores the include if the file is missing.
- `~/.config/fish/conf.d/secrets.fish`: written by the `secrets` module. See "Secrets".
- `~/.claude/settings.json`: chezmoi creates it on a new machine and never overwrites it (`create_private_settings.json.tmpl`, mode 0600), because Claude Code edits this file itself. To change the default, edit the template. An existing machine keeps its own copy.

## How to add things

Add a package. Add one line to the right file in `packages/`. `brew.txt` is for both platforms, `brew-cask-darwin.txt` is for macOS casks, `apt.txt` is for Debian and Ubuntu system packages. A `#` starts a comment. Keep the lists to core tools. Tools for one project or one environment are installed by hand.

Add a module. See "Add a module" in [AGENTS.md](AGENTS.md). In short: one file in `internal/modules`, one test file, one line in `allModules()` in `cmd/dot/main.go`, and one row in the table above and in `docs/PLAN.md`.

Add a skills profile. Add `skills/profiles/<name>.json` with the shape of a project `skills-lock.json` (version 1), and a row in [skills/README.md](skills/README.md). Then `dot skills apply <name>` works.

Add an agents template. Add `agents/<name>.md`. It starts with the content of `agents/base.md`. Then `dot agents apply <name>` works. See [agents/README.md](agents/README.md).

Change a config file. Edit it in `home/` (or through its symlink in `$HOME`). Names follow chezmoi rules: `dot_config/fish/config.fish` becomes `~/.config/fish/config.fish`. Template files (`.tmpl`) are copied, not linked.

## Tools to install by hand

These are not in `packages/brew.txt`. Install them when a project needs them:

- flyctl
- koyeb
- neonctl
- infisical
- snyk-cli
- elixir
- caddy

pnpm, bun, go, node, and rust come from mise. Python comes from conda (miniforge).

## Development

```sh
just verify        # go vet, go test, golangci-lint
go test ./...
golangci-lint run
just dry-run       # go run ./cmd/dot install --dry-run --yes
npx -y @tasks-md/lint TASKS.md
```

Every module is tested with `FakeRunner` and a fake prompter on both a darwin and a linux platform.

- [docs/PLAN.md](docs/PLAN.md): the design.
- [docs/adr/](docs/adr/): the decisions and the alternatives that were rejected.
- [TASKS.md](TASKS.md): the work queue.
- [AGENTS.md](AGENTS.md): the rules for agents and contributors.
