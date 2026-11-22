# Replay log in files

`engine/replay/` (`log.go`, `record.go`, `mem.go`, `file.go`, `reader.go`). The `replay.Log` that writes to files, and the reader of those files. What is recorded and why: [simulation](simulation.md#replay). The storage under it: [segmentlog](segmentlog.md).

## Files

| Part | Where |
|---|---|
| a log for each engine | `<dir>/<scope>-<id>/`, for example `world-1`, `zone-2`, `zonelayer-3` |
| the segments of a log | `seg-<index>.log` ([segmentlog](segmentlog.md#files)) |
| opened | by the first record of the engine; an engine that records nothing has no directory |

- An engine writes only to its own log, with no lock shared with the others. Its methods are called from the goroutine of the engine.

## Records

A record is the payload `[kind u8][tick uvarint][body]`. The whole tick is in each record, so a record does not depend on its segment. The engine is not in it: it is the engine of the log.

| Kind | Number | Body |
|---|---|---|
| `KindInbound` | 1 | sender scope `u8`, sender ID, `Seq`, count, then the number of each action ID |
| `KindCommand` | 2 | the number of the command and its fields (below) |
| `KindConfig` | 3 | the version of the config |
| `KindHash` | 4 | the hash, 8 bytes |
| `KindStableRead` | 5 | scope `u8` and ID of the engine read, the tick of the copy |

- Integers are `uvarint` unless a width is said. An address is the scope `u8` and the ID.
- An inbound message is recorded by its header and the numbers of its actions only: what the engines send is made again by a replay of the whole cluster.

## Meta of a segment

| Field | Value |
|---|---|
| data version | `uvarint`, 1: given to every `Decode` as a `binarycodec.Version` |
| engine | scope `u8`, ID |
| seed | 8 bytes |
| first tick | tick of the first record of the segment |

- The meta is made by the goroutine of the writer from the first record of the segment. `replay.ReadFileLog` starts at the last segment whose first tick is not after the tick asked for.
- A reader refuses a segment of another data version (in pre-alpha there is one) and a log of another engine.

## Numbers

Each of these is part of the format of the log: it never changes, is never used for another thing, and a new thing takes the next number. An action ID, a cooldown key and a command ID are numbers themselves (the constants of their registers), written as they are; a command writes its own fields with its method `Encode`, and a function of its file reads them, found in a table by the ID.

| What | Table | Numbers |
|---|---|---|
| action ID | `action/id.go` (the value of the constant) | 1 to 15 |
| cooldown key | `action/cooldown_key.go` (the value of the constant) | 1, 2 |
| command ID | `command/id.go` (the value of the constant) | 1 to 9 |

## Commands

| Number | Command | Fields |
|---|---|---|
| 1 | `WorldChatPrivateSendCommand` | `FromID`, `ToID`, `Text` |
| 2 | `ZoneCharacterEnterCommand` | a character (below) |
| 3 | `ZoneChatSendCommand` | `FromID`, `Text` |
| 4 | `ZoneLayerCharacterMoveCommand` | `CharacterID`, `Dir` `u8`, `Ticks` |
| 5 | `ZoneLayerCharacterNormalAttackTriggerCommand` | `CharacterID`, `TargetID` |
| 6 | `ZoneLayerChatSendCommand` | `FromID`, `Text` |
| 7 | `ZoneLayerCharacterExitToCommand` | `CharacterID`, `ExitEntityID` |
| 8 | `ZoneLayerCharacterItemPickupCommand` | `CharacterID`, `ItemEntityID` |
| 9 | `ZoneLayerCharacterShieldGrantCommand` | `CharacterID`, `Duration` |

- A string is its length and then its bytes.
- A character is `CharacterID`, `Name`, then `Health`, `MaxHealth`, `Radius` and `Speed` as signed integers, the count of cooldowns, and for each cooldown, in the order it was set, the number of its key and the tick it ends at.
- A cooldown is written as it is carried: a tick of an engine. How cooldowns follow a character between engines is in the plan of phase 1.

## Failures

| Case | Result |
|---|---|
| a record that cannot be written (a command with no codec, or whose ID is the ID of another kind) | not written; the log is marked as failed; `Err` returns the first error, which is also logged |
| a log that cannot be opened (the directory or the first segment) | the engine remembers it and does not try again for each record; `Err` returns the error, which is logged once |
| a write or a sync that fails | the segment log stops writing; `Err` and `Close` return the error |
| bytes cut off or a number that names nothing | an error from the reader, never a panic |
| a tick cut off at the end of the last segment (a crash, a process killed in the middle of a write) | not read: a tick is a batch, all or nothing, and the log ends at the last whole tick; it is not an error |

- A log that failed is not enough to replay from.

## Not here yet

- Checkpoints, the scheduler and cron in a checkpoint, the replay runner, and deleting old segments.
- A `Close` is needed to write what the last ticks recorded: until then they are in memory and on the way to disk.

Benchmarks: [replay_log](benchmark/replay_log.md).
