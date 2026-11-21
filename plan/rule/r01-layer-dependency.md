# R1. Layers depend on each other only through interfaces the caller defines (hard)

- Layers never import each other. The packages every layer may import are `config`, `shared`, and `sim` (where a layer runs or describes the simulation).
- A layer uses another layer through an interface that the **caller** defines, in the caller's own package, in the caller's own general (provider-neutral) vocabulary, with only the methods that caller uses.
- An interface is defined only when a caller needs it. There are no spare interfaces and none kept "in case". A second caller with different needs defines its own interface; an interface is never widened.
- An adapter implements the interface of a caller on top of a provider: one adapter per interface, in `adapters/<caller><provider>`. Only `cmd/*` imports adapters and wires the pieces together.
- A plug-in family (sign-in methods, effects, transports, stores) is one interface at the caller plus one package per implementation.
- Only `store` and `adapters` import `pgx` or `database/sql`.
- The rule is enforced by machine (`depguard`) as soon as project-b has a linter; until then the author keeps it.
