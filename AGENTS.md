# AGENTS.md

This repo is a public dotfiles repo, written as a Go module (`github.com/doraeminemon/dotfiles`). It sets up a new macOS or Linux machine. A Go CLI `dot` runs installer modules with a Charm TUI (`charm.land/huh/v2`). chezmoi, in symlink mode, links `$HOME` config files to `home/` in this repo.

## Read first
1. `README.md`: what the tool does and how to use it.
2. `docs/PLAN.md`: the design, layout, types, and module table.
3. `TASKS.md`: the work queue ([tasks.md spec](https://github.com/tasksmd/tasks.md)).
4. `docs/task-graph.md`: the dependency graph and the parallel waves.
5. `docs/adr/`: the decisions and the alternatives that were rejected.

## Task Management
- Pick work from `TASKS.md`: the highest priority task with no open `**Blocked by**` and no claim.
- Claim a task by adding ` (@<agent-name>)` to its task line.
- Write only to the paths in the task's `**Touches**`. Parallel agents rely on it.
- When the task is done, remove its block from `TASKS.md` in the same commit. Also remove its node from `docs/task-graph.md`.
- At tasks tagged `needs-approval`, stop and ask the user. Do not continue on your own.
- Lint the queue: `npx -y @tasks-md/lint TASKS.md`.

## Layout
| Path | What |
|---|---|
| `cmd/dot/` | CLI entry. `main.go` builds the real deps (`deps`) and `allModules()`. `install.go`, `list.go`, `project.go` are the commands. `dry.go` has `dryRunner` and `dryFS`. `repo.go` finds the checkout |
| `internal/platform` | OS / arch / package manager detection (`Platform`, `Current()`) |
| `internal/runner` | `Runner` interface (`Run`, `Output`) with `Cmd` and `CommandError`. `ExecRunner`, `DryRunner`, `FakeRunner` |
| `internal/prompt` | `Prompter` interface, `NewHuhPrompter()` (huh v2 adapter), `FakePrompter` |
| `internal/pkglist` | Parser for `packages/*.txt` |
| `internal/module` | `Module` interface, `Env`, `Registry` (`NewRegistry`, `ParseID`, `Plan`), `WriteFS`, `OSFS`, `MemFS`, typed errors |
| `internal/modules` | One installer module per file: `repo`, `apt`, `brew`, `chezmoi`, `secrets`, `fish`, `mise`, `python`, `claude`, `ssh`, `skills` |
| `internal/project` | `ParseProfile`, `ApplySkills`, `ParseTemplate`, `ApplyAgents` (`dot skills apply`, `dot agents apply`) |
| `packages/` | `brew.txt`, `brew-cask-darwin.txt`, `apt.txt`, one package per line |
| `home/` | chezmoi source (`.chezmoiroot` = `home`) |
| `skills/` | `global.skill-lock.json` and per-stack `profiles/*.json` |
| `agents/` | AGENTS.md / CLAUDE.md templates per stack |
| `docs/` | `PLAN.md`, `task-graph.md`, `adr/` |
| `.goreleaser.yaml` | static release binaries, named to match `uname` |

## Rules
- **No secrets, emails, tokens, or absolute home paths** in the repo. Personal values are chezmoi prompt data (`{{ .name }}`, `{{ .email }}`) or come from the `secrets` module, which writes `~/.config/fish/conf.d/secrets.fish` with mode 0600.
- All process execution goes through `runner.Runner`, all user input through `prompt.Prompter`, and all file writes through `module.Env.FS`. Modules must be testable with fakes on both darwin and linux `Platform` values.
- Commands that need the real terminal (sudo, chsh, ssh-keygen, gh auth) use `Cmd{Interactive: true}`. They run outside any huh form.
- Errors are typed (`runner.CommandError`, `module.UnknownModuleError`, `modules.RepoOriginMismatchError`, …) and handled with `errors.As` in `cmd/dot`.
- Charm v2 import paths are `charm.land/<pkg>/v2`. Check current APIs with `npx ctx7@latest docs /charmbracelet/huh "<topic>"` or `go doc`.
- No bootstrap script. A new machine downloads the release binary (see README). The `repo` module clones this repo, and modules read data from `RepoDir`, never from `go:embed`.
- Core package lists only. Tools for one environment (flyctl, koyeb, …) are installed by hand.

## Add a module
1. Add `internal/modules/<name>.go` with `func New<Name>(...) module.Module`. Pass injected dependencies (a clock, `os.LookupEnv`, a PATH setter) as constructor arguments, as `NewChezmoi(now)`, `NewSecrets(lookupEnv)`, and `NewBrew(prependPath)` do. Implement `ID`, `Summary`, `DependsOn`, `Supports`, `Check`, `Apply`.
2. Prefix every unexported package-level identifier in the file with the module name (`aptModule`, `brewID`, `secretsList`, `repoCloneURL`). All modules share one package, and the prefix keeps the files from colliding. Exported error types are `<Name>...Error`.
3. Add `internal/modules/<name>_test.go`. Use `runner.FakeRunner`, `prompt.FakePrompter`, and `module.MemFS`. Test a darwin and a linux `Platform`, and assert the exact commands.
4. Register it in `allModules()` in `cmd/dot/main.go`. That order breaks ties in the plan.
5. Add a row to the module table in `docs/PLAN.md` and `README.md`.

### Module rules
- `Check` changes nothing and returns `StatusMissing`, `StatusInstalled`, or `StatusUnsupported`. `Apply` runs only when `Check` says missing.
- Write files only through `env.FS` (`module.WriteFS`: `ReadFile`, `Stat`, `Lstat`, `WriteFile`, `MkdirAll`, `Rename`, `Remove`). Use `Lstat` when a symlink must not be followed.
- Dry-run invariant. `--dry-run` swaps `env.Run` for `dryRunner` and `env.FS` for `dryFS`. `dryRunner.Run` prints the command and does not run it. `dryRunner.Output` runs it (unless it is interactive). So use `Runner.Output` only for read-only queries (`brew list`, `git remote get-url`, `chezmoi diff`), and `Runner.Run` for every command that changes the system. A module that breaks this breaks `--dry-run`.
- `Check` runs in dry runs and in the module picker, so it must work with read-only commands only.

## Add a package, profile, or template
- Package: add one line to `packages/brew.txt`, `brew-cask-darwin.txt`, or `apt.txt`. Core tools only.
- Skills profile: add `skills/profiles/<name>.json` and a row in `skills/README.md`.
- Agents template: add `agents/<name>.md`, starting with the content of `agents/base.md`.

## Verify
Gate, before every commit (`just verify` runs the first three):
```sh
go vet ./... && go test ./... && golangci-lint run
go run ./cmd/dot install --dry-run --yes
npx -y @tasks-md/lint TASKS.md
```
