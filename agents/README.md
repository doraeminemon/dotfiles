# agents/

Templates for `AGENTS.md` and `CLAUDE.md` in a new project. `dot agents apply <template> [dir]` copies a template to `<dir>/AGENTS.md` and `agents/CLAUDE.md` to `<dir>/CLAUDE.md`.

| File | Use |
|---|---|
| `base.md` | Universal working rules. Every stack template starts with this content. |
| `typescript-sveltekit.md` | TypeScript, SvelteKit, Cloudflare (pnpm or bun, vitest, eslint, prettier) |
| `rust.md` | Rust (cargo, clippy, rustfmt) |
| `python.md` | Python (uv, ruff, ty, pytest) |
| `go.md` | Go (go test, golangci-lint, go vet) |
| `CLAUDE.md` | One line, `@AGENTS.md`, so Claude Code reads `AGENTS.md` |

Replace each `<placeholder>` after you copy a template. When you change `base.md`, apply the same change to the start of every stack template.
