# project-b: an online game

project-b is built to cover everything an online game needs: the simulation engine, space, characters
and combat, effects, storage, network, accounts, a client, tools and content (the phases are in
[plan/en/phases.md](plan/en/phases.md)). What exists today is the simulation core, which touches
nothing outside: no gateway, no database, no network. Outside code gives it commands and takes what
changed. It builds and its tests pass;
`go run ./server/cmd/world` runs the whole flow as a script and prints what the clients would hear; with `-replay-dir <dir>` it also writes the event log of every engine to files and reads it back.

```
make hooks   # once for each clone: runs make ci before every commit
make ci && go run ./server/cmd/world
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md). How the simulation works, the actions and the entities are in [docs/simulation.md](docs/simulation.md).

## Layout

```
docs/
  action/                           reference of the actions: index.md (conventions, priorities, index), and world.md, zone.md, zonelayer.md
  configengine.md                   the engine that owns the configuration: what it holds, how it is changed and read
  tilemap.md                        reference of TileMap: construction, Free, FreeForBody, Reach, exits
  simulation.md                     how the simulation works: platform and game, states, a tick, delivery, replay
  entity/                           reference of the entities: entity.md (the interfaces), character.md, item.md, exit.md
plan/
  rule/                             the rules (index.md)
shared/                             libraries of our own, each a package; import nothing of the project
  stablemap/                        Map: index + keys + vals, stable order of visit
  table/                            Table: a stablemap with columns of one or two keys (Index, Lookup); PriorityQueue: elements taken by priority, with the same columns
  ring/                             Ring: the last rows of a log that only grows (the chat)
  registry/                         Registry: the latest published copy under a key
  codec/                            Registry: kinds told apart by a number, each with the function that reads it
platform/                           what any game that runs in ticks is made of; imports nothing of the game
  config/                           Var[T] with defaults and limits, values parsed from JSON text, an immutable Snapshot
  configengine/                     the engine that owns the configuration: its state, the command that changes it
  sim/                              State, Action[S], Command[S], Message, Plan, Scheduler, Registry, Codecs
    onfail/                         the kinds of OnFail (retry, redirect, compensate, notify, escalate, drop)
  engine/                           Engine[S] that runs any state, the router, the store of published states, the cluster
  eventlog/                         the event log of an engine, in memory and in files; where each engine's directory is
  checkpoint/                       the checkpoint files of an engine, and the writer of them
  replay/                           plays a cluster again from the event logs and the checkpoints: the recovery line (the plan), the cut, the run
server/
  cmd/world/                        a script that runs the whole flow
  game/                             this game: the states, actions and commands, and the builder of the cluster
    sim/
      state/                        IDs of what lives in the states, Entity and the kinds, Vec (units), Rect, Dir, Audience
        world/ zone/ zonelayer/     the state of each scope, and below each an entity/ and an event/ package of its own
      action/                       every action (<scope>_<words, general to specific>_action.go), the registers, the priorities
      command/                      what comes from outside; translates into actions
      tilemap/                      the map of a zone: tiles, walls, spawn, and the exits (the zone state holds it and makes entities of them)
    testkit/                        the world, the scenario and the recorder that the tests of the game share
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md). How the simulation works, the actions and the entities are in [docs/simulation.md](docs/simulation.md).

## Not built yet

Spatial grid and sight, observers that project per viewer, the store, monsters, drops, quests, sects.
State methods and actions beyond the examples here are the owner's to define (R26).
