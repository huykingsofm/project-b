# project-b: the core of the game

The core of a game server, with nothing that touches the outside: no gateway, no database, no
network. Outside code gives it commands and takes what changed. It builds and its tests pass;
`go run ./server/cmd/world` runs the whole flow as a script and prints what the clients would hear.

```
make ci && go run ./server/cmd/world
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md).

## Layout

```
docs/
  action/                           what each action does, one file for each scope: world.md, zone.md, zonelayer.md
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
        state/                      what the scopes share: IDs, Vec, Dir, Exit
          world/ zone/ zonelayer/       the state of each scope, and below each an event/ package of its own
        action/                     every action (<scope>_<words, general to specific>_action.go), Plan, Scheduler, Cron, Context, messages, registries
        tilemap/                    the map of a zone: tiles, walls, spawn, exits (the zone state holds it)
        command/                    what comes from outside; translates into actions
      engine/                        what runs the actions: engines, router, stable, graph, replay log
```

The rules the code follows are in [plan/rule/index.md](plan/rule/index.md).

## One tick of an engine (`engine/zonelayer.go`, `Step`)

1. take the configuration once; decide on what came back undelivered;
2. build the plan: what the engine set for itself, what was scheduled for this tick (and the
   crons), the messages of other engines, the commands;
3. run the plan, one action after another, on the state, in place;
4. settle the actions that were cancelled; take what the actions set for the engine;
5. hand the events to the emitter;
6. publish a `Clone` of the state (the other engines read the latest copy; nothing is given back);
7. send the messages the actions put in the outbox; a message that cannot be delivered is decided
   on by its own `OnFail`.

## The ideas in the code

- **Scopes.** The world, a zone and a zonelayer are the three scopes. Each has a state in a package of its
  own, events in a package of its own, its own plan and its own actions.
- **State and events.** A state is data. Its methods are the only way to change it, and they write
  the events: the list of what a client should hear (`state.Events[E]` holds a `[]E`). The events are
  not cloned or published.
- **The map** belongs to the zone: the zone state holds the `tilemap.TileMap`, and a zonelayer that
  needs it reads it from the published state of its zone (`r.Stable.Zone(s.ZoneID)`). An action that
  needs the map and finds none does nothing in that tick; what it does next is its own rule (a move
  and an accept try again in the next tick, an exit request does nothing).
- **Actions never fail and never leave their scope.** What must happen outside goes in the registry
  (`ToZoneLayer`, `ToZone`, `ToWorld`, `ToSelf`, `ToSelfAt`) and happens after the plan. An action that
  must wait sets itself at the tick it knows it can run.
- **Cooldown.** A character has a cooldown for each key: the tick at which it is free of what the key
  stands for. A key is a `struct{}` type that an action defines in its own file, so keys never clash
  and any action can read another's. `ZoneLayerCharacterMoveAction` waits its turn through it. `ZoneLayerCharacterNormalAttackAction` does nothing while it is on cooldown.
- **Plan index.** The plan is indexed by the ID of its actions and by their author, so an action that
  looks at the actions behind it does not walk the plan. `ZoneLayerCharacterShieldAction` is the pattern: it stands
  in the plan every tick, finds the damage by ID and lowers or cancels the damage aimed at its owner.
- **Cancel.** An action that has something to settle when it is cancelled implements
  `ZoneLayerCancellableAction`, which is a `ZoneLayerAction` with one more method.
- **Chat.** A zonelayer message stays in the zonelayer (`ZoneLayerChatSendAction`); a zone message is recorded by the
  zone and given to each of its zonelayers (`ZoneChatSendAction`); a private message is between players and is
  the world's alone (`WorldChatPrivateSendAction`).
- **Counts are not kept.** No scope keeps a count of the characters in another: an action that needs one reads
  it from what the zonelayers published.
- **Failed delivery** is decided by the message (`OnFailRetry`, `OnFailCompensate`, ...) and run by a
  table (`engine/bounce.go`).
- **Replay** needs the inputs from outside only (`engine/replay_log.go`); what engines send themselves is
  made again from the state.

## An exit is optimistic

A character that takes an exit (`ZoneLayerCharacterExitToAction`) moves at once, when a zonelayer of the
zone the exit leads to holds fewer characters than its capacity, and does not wait for that zonelayer to
answer. The action counts from what is published (the zone's zonelayers and capacity, and how many
characters each zonelayer holds), which may be a few ticks old, so two characters that go in the same
tick may both find room for one. The character is in the message to the other zonelayer
(`ZoneLayerCharacterEnterAction`); if the message cannot be delivered, the same enter action puts the
character back where it was. When there is no room, or something it needs is not published, nothing
happens. There are two actions, exit-to and enter, and no state kept for the road between.

## Not built yet

Spatial grid and sight, observers that project per viewer, the store, monsters, items, quests, sects.
State methods and actions beyond the examples here are the owner's to define (R26).
