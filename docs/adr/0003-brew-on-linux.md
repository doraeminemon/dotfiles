# 0003. Homebrew on Linux for tools, apt for prerequisites only

## Status
Accepted, 2026-10-03.

## Context
The same tools (chezmoi, mise, starship, ripgrep, gh, and others) are needed on macOS and Linux. Ubuntu apt packages are often old or missing for these. Two tool lists, one per OS, would drift apart.

## Decision
Homebrew (Linuxbrew on Linux) installs all tools from one list, `packages/brew.txt`. macOS adds casks from `packages/brew-cask-darwin.txt`. On Linux, apt installs only the system prerequisites that Homebrew and the installer need, from `packages/apt.txt` (build-essential, curl, git, file, procps, unzip, ca-certificates, fish). Only apt-based Linux is supported now. Language runtimes (go, node, bun, pnpm, rust) come from mise, not brew.

## Consequences
- One package list to maintain for tools. A tool added once is on both OSes.
- Linux gets current versions of tools.
- Homebrew on Linux is a second package manager next to apt, and it installs under `/home/linuxbrew/.linuxbrew`.
- Linux distros without apt are unsupported until a task adds them.

## Alternatives considered
- **apt for all tools on Linux**: old versions, missing packages (chezmoi, mise, starship, ast-grep), and a second list per OS. Lost on freshness and upkeep.
- **Nix**: see ADR 0001. Lost on learning cost.
- **Per-tool install scripts and release downloads**: many code paths, each to maintain and verify. Lost on complexity.
- **mise for all tools**: fits language runtimes well, but not every CLI tool and not the system prerequisites. Kept for runtimes only.
