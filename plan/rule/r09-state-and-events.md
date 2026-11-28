# R9. State is data, the events are a region of its own, and containers keep their order

A state is data and nothing else: no function, channel or connection. Only the goroutine of the engine that owns it changes it, and only through the methods of the state, which keep the rules of the state and write the events. Each state has two regions: the **data** (the engine publishes a `Clone` of it at the end of the tick for the other engines to read) and the **events** (the list of what to tell the clients; never cloned or published). An action never makes a change itself. The state of each scope is in a package of its own (`state/world`, `state/zone`, `state/zonelayer`), and the events of each state are in a package of its own below it (`event`); each scope defines its own events, and the same name in two scopes is not a clash.

## Ordered containers

State uses `stablemap.Map`, `ring.Ring` and `table.Table`, never a plain `map`, because the iteration order of a Go map is random and breaks a replay. Every place that visits a container of the state sees the same order for the same sequence of inputs. Time is the tick count, with no system clock; randomness comes from the seeded RNG of the engine; configuration comes from a snapshot taken once at the start of the tick.
