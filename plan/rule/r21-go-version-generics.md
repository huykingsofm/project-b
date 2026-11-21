# R21. Go 1.27: use generics and the newer syntax when needed

The project uses **Go 1.27** (`go.mod` says `go 1.27`). So use generics fully (type parameters, constraints such as `comparable` or `cmp.Ordered`, generic types with methods) and the newer features of the language and the standard library when they make the code clearer: `range` over functions (`iter.Seq`, `iter.Seq2`), `min`, `max`, `clear`, the `slices` and `maps` packages, `math/rand/v2`.

- A container or mechanism that does not depend on the type of its data is generic, not `any` with a type assertion: `orderedmap.Map[K, V]`, `table.Table[T]` (rows that define `Index() map[string]any`), `plan.Plan[A]`, `schedule.Scheduler[A]`, `publish.Registry[K, T]`, `action.Message[A]`.
- Use a generic when it replaces repetition or a type assertion, not to show off. When a small interface is enough ([R1](r01-layer-dependency.md)), use the interface.
- Do not fall back to the old way out of habit: do not write your own `min`, `max`, `slices.Contains` or `maps.Clone`; do not use the old `math/rand`.
- A change of Go version, up or down, is the owner's decision and is recorded here in the same change.
