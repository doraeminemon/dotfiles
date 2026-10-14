# Type-Driven Development

Types are the specification. The implementation satisfies them. Invalid programs fail to compile instead of failing in production.

These rules apply in every language. Examples use TypeScript; use the equivalent construct in the project's language. TypeScript-specific rules live in `rules/typescript.md`.

## 1. Scope

- Build what is needed now. No speculative parameters, flags, plugin points, or abstraction layers.
- Read a dependency's installed type definitions or source before you call it. A remembered signature is a guess. For an installed dependency, its local types win over fetched docs, which may describe a different version.
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
- Do not mutate arguments. Return immutable data. Mutation is correct in infrastructure code, hot paths, and local accumulation.
- A cast lives inside a named parse or adapter function, next to the check that justifies it, with a comment that states the invariant. Never chained. Never in business logic.
- Wrap a dependency that has bad types in a thin adapter. The codebase imports the adapter.

## 4. Dependency inversion

Invert what does I/O, is non-deterministic, or crosses a process boundary: databases, HTTP clients, filesystem, clock, randomness, brokers, payments, email. Do not invert pure functions or internal helpers.

- Depend on a narrow interface that the consumer owns.
- Pass dependencies explicitly. No service locators. No ambient stateful imports.
- Domain logic must run with no network, no clock, and no database.

## 5. Workflow

1. Check the project's own modules and installed dependencies for what already exists.
2. Domain types and branded IDs.
3. Boundary schemas, inbound and outbound, and the error type.
4. Consumer-side interfaces for the inverted dependencies.
5. The signature of the work.
6. Implement.

Present steps 2 to 5 for review before you implement when the work adds a module, adds a boundary, or changes a public or persisted type. Otherwise, implement directly.

If implementation shows the types are wrong, change the types. Do not use a cast, an optional field, or a widened return.

## 6. Decision records

Records live in `docs/adr/` at the project root. Number them and never renumber. Format: Status, Context, Decision, Consequences.

- A record is due when a decision constrains future work, is expensive to reverse, or rejects a plausible alternative.
- If `docs/adr/` exists, write the record when the decision is made. If it does not exist, propose the record and ask.
- Record the alternatives and why they lost. That part has value in two years.
- Supersede, do not delete. Link to the replacement.

## 7. Calibration

Apply full rigor to public APIs, persisted data, cross-service contracts, security, money, and anything with more than one consumer. For scripts, one-off migrations, and prototypes, apply less and say so.

## 8. Existing code

Existing conventions win, even against these rules. Note the conflict once, then follow the convention. Apply these rules to new code. Apply them to existing code only on request.

@RTK.md
