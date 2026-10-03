# 0002. chezmoi in symlink mode with .chezmoiroot

## Status
Accepted, 2026-10-03.

## Context
Config files in `$HOME` (fish, starship, mise, git, Claude Code) must come from this repo. Edits to a live config should land in the repo without a copy step. The repo also holds Go code, docs, and packages that must not appear in `$HOME`.

## Decision
Use chezmoi with `[chezmoi] mode = "symlink"` and the repo as the source. `.chezmoiroot` contains `home`, so only `home/` is the chezmoi source and the rest of the repo is ignored. Files with templates (`dot_gitconfig.tmpl`, `private_dot_ssh/config.tmpl`, `dot_claude/settings.json.tmpl`) are rendered, so chezmoi copies them and does not symlink them.

## Consequences
- Editing `~/.config/fish/config.fish` edits the repo file, and the diff shows the change at once.
- Templated files are copies. An edit to the live file does not reach the repo, and the next `chezmoi apply` overwrites it. This is a known trade-off. Edit the template in `home/`.
- The `chezmoi` module backs up replaced files to `~/.dotfiles-backup/<timestamp>/` before the first apply.
- The checkout must stay on disk at `RepoDir`. If it moves, the symlinks break.

## Alternatives considered
- **chezmoi default copy mode**: the repo and `$HOME` drift apart, and every edit needs `chezmoi re-add`. Lost on workflow.
- **Plain `ln -s` script or GNU Stow**: no templates, so name, email, and OS-specific values need another mechanism. Lost on features.
- **Bare repo in `$HOME`**: no templates, no per-OS rules, and the whole home directory becomes a work tree. Lost on safety.
- **Repo root as chezmoi source (no `.chezmoiroot`)**: Go code and docs would need long ignore lists. Lost on noise.
