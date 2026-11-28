# Benchmarks of the actions

Conventions and how to run: [index](index.md). Behavior: [actions](../action/index.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkApplyZoneLayerActions/<action>` | `server/game/sim/action` | one `Apply` of each action of a zonelayer on a copy of a zonelayer with 2 characters and an item on the ground (the registry is new for each run and is emptied after it) |
| `BenchmarkApplyZoneActions/<action>` | `server/game/sim/action` | the same for each action of a zone, on a zone of 10 by 5 tiles with a zonelayer |
| `BenchmarkApplyWorldActions/<action>` | `server/game/sim/action` | the same for each action of the world, on a world with 2 zones |
| `.../the copy of the state` | `server/game/sim/action` | the copy alone (`Clone` of the state), with nothing applied |

- The copy is made for each run, because an action changes the state and the next run needs it as it was; so the number of an action holds the copy, and the table gives the copy and what is left when it is taken away.
- `ExitTo`, `ItemPickup`, `Enter`, `NormalAttackTrigger` and `ShieldGrant` take the path in which they act (they send a message, add a character, add to the plan or set themselves again), not the one in which they find nothing to do.
- The states are small: what an action costs by itself is what it does with the tables, not with the size of the state.

## Results

A zonelayer (the copy: 1,075 ns, 1,776 B, 24 allocations):

| Action | ns/op with the copy | ns/op without it | B/op | allocs/op |
|---|---|---|---|---|
| Damage | 1,268 | 193 | 1,968 | 28 |
| Enter | 1,792 | 717 | 2,640 | 35 |
| ExitTo | 2,038 | 963 | 2,560 | 38 |
| ItemPickup | 1,665 | 590 | 2,216 | 35 |
| Move | 1,608 | 533 | 2,176 | 32 |
| NormalAttackTrigger | 1,614 | 539 | 2,152 | 32 |
| Regen | 1,226 | 151 | 1,912 | 26 |
| ShieldGrant | 1,298 | 223 | 1,944 | 27 |
| AnnounceToZone | 1,262 | 187 | 2,016 | 29 |
| ChatSend | 1,256 | 181 | 1,968 | 28 |
| ItemEntityAdd | 1,473 | 398 | 2,160 | 32 |
| ItemEntityRemove | 1,374 | 299 | 1,920 | 27 |

A zone (the copy: 248 ns, 456 B, 5 allocations):

| Action | ns/op with the copy | ns/op without it | B/op | allocs/op |
|---|---|---|---|---|
| CharacterEnter | 475 | 227 | 792 | 10 |
| ChatSend | 522 | 274 | 784 | 13 |
| ExitAdd | 584 | 336 | 896 | 13 |
| ZoneLayerAdd | 406 | 158 | 616 | 8 |

The world (the copy: 106 ns, 192 B, 2 allocations):

| Action | ns/op with the copy | ns/op without it | B/op | allocs/op |
|---|---|---|---|---|
| CharacterItemAdd | 682 | 577 | 1,064 | 15 |
| ChatPrivateSend | 254 | 149 | 416 | 6 |
| ZoneAdd | 294 | 188 | 641 | 6 |
| ZoneExitAdd | 251 | 146 | 448 | 7 |
| ZoneLayerAdd | 473 | 368 | 928 | 12 |

## Reading

- **An action costs 0.15 to 1 µs** by itself, against the 45 ms of a tick: a tick of 1,000 actions is under 1 ms. The ones that read other engines (`ExitTo`, `Enter`) and that work out where the character can stand (`Move`, `NormalAttackTrigger`, `ItemPickup`) are the dearest.
- **The allocations are the registry and what it sends:** each message and each action sent is a value in an interface, so `Send` of one action is 3 to 6 allocations. `Regen` and `Damage`, which send nothing, allocate the least.
- **The number without the copy is a difference of two numbers** of 1 to 2 µs each: it is good to some 50 ns, and a smaller difference says nothing.
