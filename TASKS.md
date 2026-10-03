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

- [ ] Keep machine-only config out of chezmoi overwrites (@w7-local)
  - **ID**: local-overrides
  - **Tags**: chezmoi, ssh, claude
  - **Estimate**: 30m
  - **Surfaced-by**: migrate-this-mac diff review (user decision 2026-10-03)
  - **Details**: (1) `home/private_dot_ssh/config.tmpl`: first line `Include ~/.ssh/config.local` (ssh ignores a missing Include file), so private hosts live in an untracked `~/.ssh/config.local`. (2) Rename `home/dot_claude/settings.json.tmpl` to `home/dot_claude/create_settings.json.tmpl`, so chezmoi creates it on new machines and never overwrites an existing one (Claude Code edits this file itself). Check the chezmoi `create_` + `.tmpl` attribute order with `chezmoi help` or the docs. (3) README: a short "Machine-only config" section covering `~/.ssh/config.local`, `secrets.fish`, and that settings.json is create-only.
  - **Touches**: `home/private_dot_ssh/config.tmpl`, `home/dot_claude/`, `README.md`
  - **Acceptance**: `chezmoi --source . execute-template` renders the ssh config with the Include first. `chezmoi --source . managed` still lists `.claude/settings.json`. The gate passes.

- [ ] Migrate this Mac to the chezmoi symlinks
  - **ID**: migrate-this-mac
  - **Tags**: needs-approval, chezmoi
  - **Estimate**: 20m
  - **Details**: Run `go run ./cmd/dot install --only chezmoi`. Review the diff with the user, confirm the backup, and apply. Check that `ls -l ~/.config/fish/config.fish` points into the repo and that a new fish shell starts with no errors.
  - **Touches**: (none in repo — changes $HOME)
  - **Acceptance**: The user approved and the symlinks are in place. The backup dir is listed in the report.
  - **Blocked by**: local-overrides

## P2

- [ ] Smoke-test bootstrap on a fresh Ubuntu VM
  - **ID**: linux-smoke
  - **Tags**: needs-approval, linux, test
  - **Estimate**: 45m
  - **Details**: `orb create ubuntu dottest`. Inside it, run the README curl one-liner (no git or Go installed beforehand), then `./dot doctor`. Delete the VM after. Record the failures as new tasks.
  - **Touches**: (none — VM only)
  - **Acceptance**: `dot doctor` shows every Linux-supported module as Installed.
