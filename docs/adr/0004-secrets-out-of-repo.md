# 0004. Secrets never in the repo

## Status
Accepted, 2026-10-03.

## Context
The repo is public. Some config needs secret or personal values: the Exa API key (`EXA_API_KEY`), the user name and email, and tokens for tools such as npm and snyk. A secret pushed to a public repo cannot be taken back.

## Decision
No secret, token, email, or absolute home path is stored in the repo. Personal values for templates (name, email) come from chezmoi prompt data. API keys are asked for by the `secrets` module at install time with masked input. In non-TTY mode the module reads `DOT_<VAR>` environment variables. The module writes `~/.config/fish/conf.d/secrets.fish` with mode 0600, through the `WriteFS` interface. chezmoi does not manage this file, and `.gitignore` excludes `secrets.fish`, `.env*`, and `*.local.*`. SSH private keys and `~/.npmrc` are never read or copied. Values are never logged. A gitleaks scan and a grep for personal strings must pass before any push.

## Consequences
- A new machine needs the user to type or provide each secret once.
- Secrets are not backed up or synced by this repo. Keep them in a password manager.
- The secret file is a plain file at mode 0600, not an encrypted store.
- CI runs gitleaks to catch mistakes.

## Alternatives considered
- **Commit encrypted secrets (age, sops, chezmoi encryption)**: a public repo would hold ciphertext forever, and a key leak exposes all history. Lost on risk.
- **Password manager integration (1Password, Infisical, chezmoi password-manager functions)**: needs that tool installed and signed in before the secrets step, and ties the repo to one vendor. Lost on bootstrap cost.
- **Environment variables only, no file**: the user must set them again in each new shell. Lost on usability. Kept only as the non-TTY input path.
- **A chezmoi template that holds the values**: the values would sit in the repo or its state. Lost on exposure.
