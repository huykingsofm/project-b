# Checkpoint of an engine

`engine/checkpoint.go`, with the codecs of the states (`sim/state/*/codec.go`), of the entities (`sim/state/entity/codec.go`) and of the scheduler (`sim/action/scheduler_codec.go`). What an engine needs to go on from the tick it was taken at, as bytes. The design: `plan/en/proposed-designs.md`.

## Entry points

| Call | What it does |
|---|---|
| `WorldEngine.Checkpoint()`, `ZoneEngine.Checkpoint()`, `ZoneLayerEngine.Checkpoint()` | writes the checkpoint of the engine; called between two ticks, by the goroutine of the engine or while it does not run |
| `RestoreWorldEngine(data, env, seed)` | makes the engine of the world from a checkpoint, at its tick |
| `RestoreZoneEngine(data, env, seed, tilesOf)` | the same for a zone; `tilesOf` gives the map of the zone by its ID |
| `RestoreZoneLayerEngine(data, env, seed)` | the same for a zonelayer |

- A restore registers the engine in the router of `env`, as `New...Engine` does.
- What an engine does again and again (a zonelayer healing its characters) is an action that sets itself again, so it waits in the scheduler and is in the checkpoint like any other. `ZoneLayerEngine.EnsureRegen(every)` checks it after a restore: the action that waits must have the period the cluster asks for, and a different one is an error.
- A checkpoint that is cut, has a byte too many, is of another data version or of another scope is an error, never a panic.

## What is written

In this order:

| Part | Holds |
|---|---|
| version | the version of the data, `uvarint` (1) |
| address | scope `u8` and ID of the engine; the restore checks it against the scope and the state |
| tick | the tick the engine is at |
| state | the data of the state, by its `Encode` (below) |
| scheduler | the last tick it gave out, and the actions set for later ticks, by tick and in order |
| self list | what the engine set for itself for the next tick |
| random numbers | where the generator of the engine is (the PCG), as its own bytes |
| `seq` | the number of the last message the engine sent |
| configuration | the version of the configuration the engine first read last |
| `lastSeq` | the last number taken from each sender, in the order of the addresses |
| messages held | the messages the bouncer holds to send again, each written whole |

## What is not written

| Left out | Why |
|---|---|
| the events of the state | they are not data (R18); the engine gives them to its clients at the end of the tick |
| the map of a zone | it never changes, and the ID of the zone finds it |
| the queues of the engine | what was not yet taken from them is in the replay log |
| `View`, the registry of the tick, the emitters | made again by the engine |

## State, by scope

| State | Data written |
|---|---|
| world | ID, tick, the private messages, the inventory of each character |
| zone | ID, tick, its entities (the exits), the zonelayers it knows with their capacity, what was said, the next entity ID |
| zonelayer | IDs, tick, its entities, its characters with their cooldowns, what was said, which character has which body, the next entity ID |

- An entity is written as its kind (`entity.KindID`) and then what it is made of; each kind has its decoder in `entity/codec.go`, and a test fails when a kind of the register has none.
- A count that claims more items than the bytes left can hold is refused before anything is made.

## Hash

- The hash of an engine is the hash of its state and of what it waits for: the scheduler and the self list, each action as the bytes it is written as, so a field only the action knows (a shield that started) changes it.
- A restore that waits for something else than the engine it came from has another hash, and a replay finds it.

## Not here yet

- The copy taken at the end of the tick and written outside the goroutine of the engine, the period with an offset for each engine, and the files: the next part of the plan.
- Loading a set of checkpoints for the cluster.

Benchmarks: [checkpoint](benchmark/checkpoint.md).
