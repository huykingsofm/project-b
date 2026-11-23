# Checkpoint of an engine

`platform/engine/checkpoint.go`, with the codecs of the states (`server/game/sim/state/*/codec.go`), of the entities (`server/game/sim/state/zonelayer/entity/codec.go` and `server/game/sim/state/zone/entity/codec.go`) and of the scheduler (`platform/sim/scheduler.go`). What an engine needs to go on from the tick it was taken at, as bytes. The design: `plan/en/proposed-designs.md`.

## Entry points

| Call | What it does |
|---|---|
| `Engine[S].Checkpoint()` | writes the checkpoint of the engine; called between two ticks, by the goroutine of the engine or while it does not run |
| `engine.Restore[S](data, env, seed)` | makes the engine of a state of type `S` from a checkpoint, at its tick; the state is read with `env.Codecs`, which for a zone knows the map of the zone (`game.NewCodecs(tilesOf)`) |

- A restore registers the engine in the router of `env`, as `engine.New` does, and only after all of the checkpoint has been read and checked: a bad checkpoint returns an error and registers nothing.
- What an engine does again and again (a zonelayer healing its characters) is an action that sets itself again, so it waits in the scheduler and is in the checkpoint like any other. `game.EnsureRegen(engine, every)` checks it after a restore: the action that waits must have the period the cluster asks for, and a different one is an error.
- A checkpoint that is cut, has a byte too many, is of another data version or of another type of state is an error, never a panic.

## What is written

In this order:

| Part | Holds |
|---|---|
| version | the version of the data, `uvarint` (1) |
| state ID | the ID of the engine, `uvarint`; its high 8 bits are the type of the state, which the restore checks against the type it was asked for, and the ID against the state |
| tick | the tick the engine is at |
| state | the data of the state, by its `Encode` (below) |
| scheduler | the last tick it gave out, and the actions set for the ticks to come (the next one included), by tick and in order |
| random numbers | where the generator of the engine is (the PCG), as its own bytes |
| `seq` | the number of the last message the engine sent |
| configuration | the version of the configuration the engine first read last |
| `lastSeq` | the last number taken from each sender, in the order of the state IDs |
| messages held | the messages the bouncer holds to send again, each written whole |

## What is not written

| Left out | Why |
|---|---|
| the events of the state | they are not data (R18); the engine gives them to its clients at the end of the tick |
| the map of a zone | it never changes, and the ID of the zone finds it |
| the queues of the engine | what was not yet taken from them is in the replay log |
| `View`, the registry of the tick, the emitters | made again by the engine |

## State, by type

| State | Data written |
|---|---|
| world | ID, tick, every zone and zonelayer, the private messages, the inventory of each character |
| zone | ID, world ID, tick, the zones its exits lead to, its entities (the exits), the zonelayers it knows with their capacity, what was said, the number the next entity gets |
| zonelayer | IDs (zonelayer, zone, world), tick, its entities, its characters with their cooldowns, what was said, which character has which body, the number the next entity gets |

- An entity of a zonelayer is written as its kind (`KindID`) and then what it is made of; each kind has its decoder in the `codec.go` of the `entity` package of its state, and a test fails when a kind of the register has none. An entity writes only the number its state gave it, not the whole `EntityID`: the state is the owner of all its entities and gives its own ID back when it reads them (the bodies of the characters are written the same way), and a state that holds an entity of another owner cannot be written. An entity is written once, not under a key.
- A count that claims more items than the bytes left can hold is refused before anything is made.

## Hash

- The hash of an engine covers what the checkpoint of it holds, which is what the engine goes on from:

| Part of the hash | How |
|---|---|
| state | the hash of the state |
| scheduler, self list | each action as the bytes it is written as, so a field only the action knows (a shield that started) changes it |
| what only the engine knows | the bytes the checkpoint writes for it: random numbers, `seq`, `lastSeq`, the version of the configuration, the messages held |

- A restore that differs from the engine it came from in any of them has another hash, and a replay finds it.

## Not here yet

- The copy taken at the end of the tick and written outside the goroutine of the engine, the period with an offset for each engine, and the files: the next part of the plan.
- Loading a set of checkpoints for the cluster.

Benchmarks: [checkpoint](benchmark/checkpoint.md).
