# AGENTS.md

Rules for any coding agent working in this repo. They are mandatory.

## Principles

- Types are the specification. Make the type checker find undefined behavior at compile time. Define every type as narrowly as possible. Do not widen types. Do not use multi-step casts such as `as unknown as A`.
- Do not use `any` (or the equivalent in your language). It hides undefined behavior.
- Make illegal states unrepresentable. Use a discriminated union, not a flag with optional fields.
- Parse external data once at the edge (HTTP, env vars, files, CLI args, database rows, third-party SDK returns). Downstream code does not re-check it.
- Return expected failures as values in the domain. Throw only at the edge.
- Build what is needed now. Do not add speculative flags, parameters, or abstraction layers.
- Check the project's own modules and installed dependencies before you write new code. Read a dependency's type definitions before you call it.

## Dependency inversion

- Invert what does I/O, is non-deterministic, or crosses a process boundary: database, HTTP client, filesystem, clock, randomness, network.
- The consumer owns the interface. Define it next to the code that calls it, with only the members that code uses.
- Pass dependencies through constructors or parameters. No service locators. No ambient global state.
- Do not invert pure functions or internal helpers.

## Lint and checks

- Do not disable a lint rule by any means: no ignore comments, per-file ignores, or config changes to make a check pass. Fix the root cause. If a rule is wrong for this repo, stop and ask.
- Run the full verify command before you report a task as done. Report the real output. Never report success without a passing run.

## Naming

- Do not name a file, directory, or function `shared`, `common`, `helpers`, or `utils`. Find the name that says what the code is. Put code in its domain, and name the file after the function when it is the only one.

## Workflow

For each non-trivial task, do the steps in order. State the current step in your messages.

1. Read the request and the existing code you will touch.
2. Define the types, interfaces, and function signatures.
3. Write failing tests against them. Each test must fail on an assertion, not on an import or syntax error. Use in-memory fakes, not mocks of concrete classes.
4. Implement the smallest code that makes the tests pass. Do not change the types or tests in this step. If they are wrong, go back to the earlier step and say why.
5. Verify with the full verify command.

For a small task, you can combine the steps. If a later step shows an earlier one was wrong, go back and change it.

## Secrets and scratch files

- `.env` files hold secrets. Do not read them directly. Use the project's env tool to inject variables into a process.
- Use `/tmp` or `./tmp` as a scratch pad for experiments.

## Decisions and docs

- Write an Architecture Decision Record in `docs/adr/` when a decision constrains future work, is expensive to reverse, or rejects a plausible alternative. Number the records and never renumber them. Format: Status, Context, Decision, Consequences. Record the alternatives and why they lost. Supersede a record, do not delete it.
- Write docs in Simple Technical English: short sentences, one idea per sentence, active voice.

## Git

- Do not commit unless the user asks. Never push, open pull requests, or merge unless the user asks.
- Keep commits small and focused. Use a clear imperative summary line.

## Task management

- Work comes from `TASKS.md` when the repo has one ([tasks.md spec](https://github.com/tasksmd/tasks.md)). Read it before you ask the user for work.
- Claim a task by adding ` (@<agent-name>)` to its task line before you start.
- Write only to the paths in the task's `**Touches**`.
- Remove a completed task from the file in the same commit. History is in `git log`.

## Stack: Rust

- Use `cargo` for everything. Pin the toolchain in `rust-toolchain.toml`.
- Model states as enums, not flags. Use newtypes for identifiers and units. Construct them through a parse function that returns `Result`.
- Errors are values. Library code returns `Result<T, E>` with a typed error enum (`thiserror`). Application code may use `anyhow` at the edge only. Do not use `unwrap()` or `expect()` outside tests. Do not panic on external input.
- Parse external data once at the edge (`serde`, with `deny_unknown_fields` where the boundary is strict). Downstream code takes typed values.
- Define traits in the consuming module, with only the methods it calls. Pass dependencies as generics or `&dyn Trait` arguments. No global mutable state.
- Do not use `unsafe` unless you must. If you do, add a `// SAFETY:` comment that states the invariant.
- Do not add `#[allow(...)]` to silence clippy. Fix the cause.
- Tests: unit tests in the module under `#[cfg(test)]`, integration tests in `tests/`. Use fakes that implement the trait.

### Verify

```sh
cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test
```
