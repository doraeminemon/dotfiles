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

## P2

- [ ] Smoke-test bootstrap on a fresh Ubuntu VM
  - **ID**: linux-smoke
  - **Tags**: needs-approval, linux, test
  - **Estimate**: 45m
  - **Details**: `orb create ubuntu dottest`. Inside it, run the README curl one-liner (no git or Go installed beforehand), then `./dot doctor`. Delete the VM after. Record the failures as new tasks.
  - **Touches**: (none — VM only)
  - **Acceptance**: `dot doctor` shows every Linux-supported module as Installed.
