# R24. No aliases (hard)

A name is never defined as another name for something already defined: not `type ZoneLayerAction = sim.Action[*zonelayerstate.State]`, not a generic alias such as `type ZoneLayerPlan = sim.Plan[*zonelayerstate.State]`, not a `var` or a `func` that only renames another one. The place where a thing is defined must be found at once from where it is used; a short alias is never worth a lookup.

Write the real name where it is used, even when it is long. Use `sim.Action[*zonelayerstate.State]` and `*sim.Plan[*zonelayerstate.State]` directly.

A defined type that is really a new type, such as `type ID interface{}`, is not an alias. An import alias (`zonelayerstate "…/state/zonelayer"`) only picks a local name for a package and is allowed.
