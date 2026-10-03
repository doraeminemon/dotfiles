# Tasks

<!-- policy: Read docs/PLAN.md and AGENTS.md before you start any task. docs/task-graph.md shows the dependency graph and the parallel waves.
     policy: Modify only the paths in the task's Touches. If the task needs another path, stop and report it. Do not widen the write-set yourself.
     policy: Never commit secrets, tokens, email addresses, or absolute home paths. Use chezmoi template data or $HOME.
     policy: Never change the real $HOME, run sudo, or push to GitHub unless the task is tagged needs-approval and the user approved it in this session.
     policy: Go code must pass go vet ./..., go test ./..., and golangci-lint run before the task is done.
     policy: Read a dependency's current API (ctx7 or go doc) before you call it. Charm v2 imports are charm.land/<pkg>/v2.
     policy: Remove the task block from this file in the same commit that completes it. -->

## P1

<!-- Wave 0: no blockers. All can run in parallel. -->

<!-- Wave 1: needs go-scaffold. -->

<!-- Wave 2. -->

<!-- Wave 3: modules. Each touches only its own file and test, so all can run in parallel. Each exports New<Name>() module.Module; cli-commands registers them. -->

<!-- Wave 4. -->

<!-- Wave 5. -->

- [ ] Load the conda hook from config.fish instead of running conda init (@w8-conda)
  - **ID**: conda-hook
  - **Tags**: go, python, fish, bug
  - **Estimate**: 30m
  - **Surfaced-by**: migrate-this-mac diff review: `conda init fish` had appended two blocks with absolute paths to config.fish
  - **Details**: In symlink mode, `conda init fish` would write machine-specific absolute paths into the repo's config.fish. (1) python module: remove the `conda init fish` step and keep `conda config --set auto_activate_base false`. (2) `home/dot_config/fish/config.fish`: add a portable block that loops over `/opt/homebrew/Caskroom/miniforge/base`, `/usr/local/Caskroom/miniforge/base`, `$HOME/miniforge3`; for the first one where `bin/conda` is executable, run `eval $base/bin/conda shell.fish hook | source`, then break. Check it with `fish -n`. (3) README "Machine-only config": add that `~/.config/fish/conf.d/local.fish` is untracked and is the place for private env vars.
  - **Touches**: `internal/modules/python.go`, `internal/modules/python_test.go`, `home/dot_config/fish/config.fish`, `README.md`
  - **Acceptance**: Tests updated so no `conda init` call remains. `fish -n` passes. The gate passes.

- [ ] Migrate this Mac to the chezmoi symlinks
  - **ID**: migrate-this-mac
  - **Tags**: needs-approval, chezmoi
  - **Estimate**: 20m
  - **Details**: Run `go run ./cmd/dot install --only chezmoi`. Review the diff with the user, confirm the backup, and apply. Check that `ls -l ~/.config/fish/config.fish` points into the repo and that a new fish shell starts with no errors.
  - **Touches**: (none in repo — changes $HOME)
  - **Acceptance**: The user approved and the symlinks are in place. The backup dir is listed in the report.
  - **Blocked by**: conda-hook

## P2

- [ ] Smoke-test bootstrap on a fresh Ubuntu VM
  - **ID**: linux-smoke
  - **Tags**: needs-approval, linux, test
  - **Estimate**: 45m
  - **Details**: `orb create ubuntu dottest`. Inside it, run the README curl one-liner (no git or Go installed beforehand), then `./dot doctor`. Delete the VM after. Record the failures as new tasks.
  - **Touches**: (none — VM only)
  - **Acceptance**: `dot doctor` shows every Linux-supported module as Installed.
