# project-b: an online game

project-b is built to cover everything an online game needs: the simulation engine, space, characters
and combat, effects, storage, network, accounts, a client, tools and content (the phases are in
[plan/en/phases.md](plan/en/phases.md)). What exists today is the simulation core, which touches
nothing outside: no gateway, no database, no network. Outside code gives it commands and takes what
changed. It builds and its tests pass;
`go run ./server/cmd/world` runs the whole flow as a script and prints what the clients would hear; with `-replay-dir <dir>` it also writes the replay log of every engine to files and reads it back.

```
make hooks   # once for each clone: runs make ci before every commit
make ci && go run ./server/cmd/world
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md). How the simulation works, the actions and the entities are in [docs/simulation.md](docs/simulation.md).

## Layout

```
docs/
  action/                           reference of the actions: index.md (conventions, priorities, index), and world.md, zone.md, zonelayer.md
  tilemap.md                        reference of TileMap: construction, Free, FreeForBody, Reach, exits
  simulation.md                     how the simulation works: scopes, state and events, a tick, delivery, replay
  entity/                           reference of the entities: entity.md (the interfaces), character.md, item.md, exit.md
plan/
  rule/                             the rules (index.md)
server/
  cmd/world/                        a script that runs the whole flow
  internal/
    config/                         Var[T] with defaults and limits, values parsed from JSON text, an immutable Snapshot
    shared/                         libraries of our own, each a package; import nothing of the project
      orderedmap/                   Map: index + keys + vals, fixed iteration order
      table/                        Table: rows that define Index(), dirty, FindBy(column, value)
      registry/                     Registry: the latest published copy under a key
    services/game/
      sim/                          the whole game, no engine talks to another, nothing outside
        state/                      what the scopes share: IDs, Vec (units), Rect, Dir
          entity/                   the entities of a zonelayer: character, item, exit
          world/ zone/ zonelayer/       the state of each scope, and below each an event/ package of its own
        action/                     every action (<scope>_<words, general to specific>_action.go), Plan, Scheduler, Context, messages, registries
        tilemap/                    the map of a zone: tiles, walls, spawn, and the exits as entities (the zone state holds it and makes them entities)
        command/                    what comes from outside; translates into actions
      engine/                        what runs the actions: engines, router, stable, graph, replay log
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md). How the simulation works, the actions and the entities are in [docs/simulation.md](docs/simulation.md).

## Not built yet

Spatial grid and sight, observers that project per viewer, the store, monsters, drops, quests, sects.
State methods and actions beyond the examples here are the owner's to define (R26).
