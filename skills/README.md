# Skills

Agent skills are managed with the [vercel-labs `skills` CLI](https://github.com/vercel-labs/skills).

- `global.skill-lock.json`: the global skills lock (copy of `~/.agents/.skill-lock.json`, lock version 3). The `skills` install module restores it.
- `profiles/<name>.json`: per-stack locks. Each has the same shape as a project `skills-lock.json` (version 1). A skill can be in more than one profile.

## Profiles

| Profile | Skill | Source |
|---|---|---|
| sveltekit-cloudflare | svelte-code-writer | sveltejs/ai-tools |
| | svelte-core-bestpractices | sveltejs/ai-tools |
| | tailwind-4-docs | Lombiq/Tailwind-Agent-Skills |
| | color-expert | meodai/skill.color-expert |
| | ux-expert | felixgeelhaar/skills |
| | vitest | antfu/skills |
| typescript-strict | typescript | thaind97git/best-practices |
| | typescript-design-patterns | peterbamuhigire/skills-web-dev |
| | typescript-effective | peterbamuhigire/skills-web-dev |
| | typescript-strict | citypaul/.dotfiles |
| | functional | citypaul/.dotfiles |
| | domain-driven-design | citypaul/.dotfiles |
| | integrate-valibot | sandros94/open-circle-utils |
| | vitest | antfu/skills |
| neon | neon | neondatabase/agent-skills |
| | neon-postgres | neondatabase/agent-skills |
| | database-schema-designer | borghei/Claude-Skills |
| rust | rust | pvillega/claude-templates |
| | rust-type-system | mhbzhy-lost/claude-personal-config |
| | tdd-red-green-refactor | google-labs-code/stitch-sdk |
| python-uv | autoresearch | uditgoenka/autoresearch |
| | exa-contents | exa-labs/agent-skills |
| | exa-search | exa-labs/agent-skills |
| | lint-tasks | tasksmd/tasks.md |
| | next-task | tasksmd/tasks.md |
| | typst-writing-document | Myriad-Dreamin/tinymist |
| go | cli-design | citypaul/.dotfiles |
| | color-expert | meodai/skill.color-expert |
| | tdd-red-green-refactor | google-labs-code/stitch-sdk |
| product | jobs-to-be-done | deanpeters/Product-Manager-Skills |
| | lean-ux-canvas | deanpeters/Product-Manager-Skills |
| | prd-development | deanpeters/Product-Manager-Skills |
| | mermaid-expert | sickn33/antigravity-awesome-skills |
| | api-and-interface-design | v1truv1us/ai-eng-system |

Notes:
- `color-expert` and `ux-expert` were local copies in one project. Their sources are the upstream repos above.
- The profiles are groupings of skills that were found in existing project locks. Edit them as the stacks change.

## Apply

```sh
dot skills apply <profile> [dir]
```

This copies `profiles/<profile>.json` to `<dir>/skills-lock.json` (default: the current directory) and runs the skills CLI restore. The command is implemented in a later task (`project-apply`).
