---
paths:
  - "**/*.ts"
  - "**/*.tsx"
  - "**/*.mts"
  - "**/*.cts"
---

# TypeScript

Follow the project's linter and tsconfig. Where they are absent or looser, still write code that passes `strict`, with no `any` and exhaustive matches.

- No widening. Use `satisfies` instead of an annotation that collapses literal inference.
- Return `readonly` data.

## Effect

Where the project uses Effect, use its parts instead of reimplementing them.

- `Schema` for boundary parsing and derived types, both directions.
- `Effect<A, E, R>` as the error channel. Typed errors as `Data.TaggedError`, handled with `catchTag`.
- `Context.Tag` and `Layer` for dependency inversion. The `R` channel makes a missing dependency a compile error.
- `Ref` for state, not mutable module scope.
- Run at the edge with `runPromise` or `runMain`.

Ask before you add Effect to a project that does not use it. Elsewhere, use zod or valibot and a plain `Result` union.
