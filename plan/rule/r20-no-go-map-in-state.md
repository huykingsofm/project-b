# R20. No Go `map` in state; iteration order is fixed

State uses `orderedmap.Map` and `table.Table`, never a plain `map`, because the iteration order of a Go map is random and breaks a replay. Every place that visits a container of the state sees the same order for the same sequence of inputs. Time is the tick count, with no system clock; randomness comes from the seeded RNG of the engine; configuration comes from a snapshot taken once at the start of the tick.
