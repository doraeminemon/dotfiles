# Type-Driven Development

Types are the specification. The implementation satisfies them. Invalid programs fail to compile instead of failing in production.

The linter and tsconfig enforce the mechanical rules: no `any`, exhaustive matches, strict flags. Do not restate them. Do not violate them.

## 1. Scope

- Build what is needed now. No speculative parameters, flags, plugin points, or abstraction layers.
- Check the project's own modules and its installed dependencies before you write new code.
- Read a dependency's type definitions before you call it. A remembered signature is a guess.
- A new dependency costs supply chain, size, and maintenance. Add one when it removes real complexity.
- YAGNI governs features and abstraction, not type precision. A branded ID or a parser specifies what exists today.

## 2. Boundaries

Everything outside the program is untrusted. Parse it once at the edge. Do not re-check it downstream.

Boundaries: HTTP requests and responses, env vars, config, CLI args, `JSON.parse`, database rows, queue payloads, webhooks, third-party SDK returns, files, stdin, FFI.

- Parse, do not validate. Write `parseUser(input: unknown): User`, not a boolean check and a cast.
- Derive the type from the schema. A hand-written type and a separate validator drift apart.
- Narrow to the domain, not to the sample. Do not close an enum over values an upstream service can extend.
- Strip unknown fields. Fail closed at security and financial boundaries.
- Map outbound. Domain types do not reach API responses, logs, or storage.
- A defensive re-check deep in the call stack means the boundary is in the wrong place.

## 3. Modeling

- Make illegal states unrepresentable. Use a discriminated union, not a flag and conditional optionals. If a comment must explain which fields go together, the type is wrong.
- Brand identifiers and units. `UserId` is not `OrderId` is not `string`. Construct through a parse function.
- Errors are values in the domain. Validation, not-found, forbidden, and timeout belong in the return type. Throw at the boundary only.
- Prefer total functions. Narrow the input until guards become unnecessary.
- Do not mutate arguments. Return `readonly` data. Mutation is correct in infrastructure code, hot paths, and local accumulation.

## 4. Type discipline

- No widening. Use `satisfies` instead of an annotation that collapses literal inference.
- A cast lives inside a named parse or adapter function, next to the check that justifies it, with a comment that states the invariant. Never chained. Never in business logic.
- Wrap a dependency that has bad types in a thin adapter. The codebase imports the adapter.

## 5. Effect

Where the project uses Effect, use its parts instead of reimplementing them.

- `Schema` for boundary parsing and derived types, both directions.
- `Effect<A, E, R>` as the error channel. Typed errors as `Data.TaggedError`, handled with `catchTag`.
- `Context.Tag` and `Layer` for dependency inversion. The `R` channel makes a missing dependency a compile error.
- `Ref` for state, not mutable module scope.
- Run at the edge with `runPromise` or `runMain`.

Ask before you add Effect to a project that does not use it. Elsewhere, use zod or valibot and a plain `Result` union.

## 6. Dependency inversion

Invert what does I/O, is non-deterministic, or crosses a process boundary: databases, HTTP clients, filesystem, clock, randomness, brokers, payments, email. Do not invert pure functions or internal helpers.

- Depend on a narrow interface that the consumer owns.
- Pass dependencies explicitly. No service locators. No ambient stateful imports.
- Domain logic must run with no network, no clock, and no database.

## 7. Workflow

1. Check what already exists.
2. Domain types and branded IDs.
3. Boundary schemas, inbound and outbound, and the error type.
4. Consumer-side interfaces for the inverted dependencies.
5. The signature of the work.
6. Implement.

Present steps 2 to 5 for review before substantial implementation. Skip the review when the task is small.

If implementation shows the types are wrong, change the types. Do not use a cast, an optional field, or a widened return.

## 8. Decision records

Records live in `/docs/adr/` of the project. Number them and never renumber. Format: Status, Context, Decision, Consequences.

- Write one when a decision constrains future work, is expensive to reverse, or rejects a plausible alternative.
- Record the alternatives and why they lost. That part has value in two years.
- Write it when the decision is made.
- Supersede, do not delete. Link to the replacement.

## 9. Calibration

Apply full rigor to public APIs, persisted data, cross-service contracts, security, money, and anything with more than one consumer. For scripts, one-off migrations, and prototypes, apply less and say so.

## 10. Existing code

Existing conventions win, even against these rules. Note the conflict once, then follow the convention. Apply these rules to new code. Apply them to existing code only on request.

@RTK.md
