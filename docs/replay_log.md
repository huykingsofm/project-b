# Replay log in files

`platform/replay/` (`log.go`, `record.go`, `mem.go`, `file.go`, `reader.go`). The `replay.Log` that writes to files, and the reader of those files. What is recorded and why: [simulation](simulation.md#replay). The storage under it: [segmentlog](segmentlog.md).

## Files

| Part | Where |
|---|---|
| a log for each engine | `<dir>/<name>-<n>/`: the name of the type of its state (`State.String()`) and the number of the state among the states of its type, for example `world-1`, `zone-2`, `zonelayer-1`; the meta of the segments holds the whole ID |
| the segments of a log | `seg-<index>.log` ([segmentlog](segmentlog.md#files)) |
| opened | by the first record of the engine; an engine that records nothing has no directory |

- An engine writes only to its own log, with no lock shared with the others. Its methods are called from the goroutine of the engine.

## Writing

| Method of `replay.Log` | What it does |
|---|---|
| `Wants(kind)` | says whether the log wants records of the kind; an engine asks before it takes a hash, which costs as much as the state is big |
| `Describe(engine, type, name)` | told once by an engine before it records: which type of state it has and what the type is called, for the name of its directory and for the meta |
| `Append(engine, tick, body)` | records what the engine took, sent or read at the tick |
| `EndTick(engine, tick)` | says that the engine has recorded all it will for the tick; a log in files hands the records to the goroutine that writes them |

## Records

A record is the payload `[kind u8][tick uvarint][body]`. The whole tick is in each record, so a record does not depend on its segment. The engine is not in it: it is the engine of the log.

| In code | What it is |
|---|---|
| `replay.Record` | `Engine`, `Tick` and a `Body` |
| `replay.Body` | an interface: `Kind()`, `Encode(w)` and `Same(other)` |
| a type for each kind | `Inbound`, `Command`, `ConfigRead`, `Hash`, `StableRead`, `Returned`: each holds only what its kind has |
| `decoders` | the table that reads a body, by its kind; a test fails when a kind of the register has none |

| Kind | Number | Body |
|---|---|---|
| `KindInbound` | 1 | a message, whole (below) |
| `KindCommand` | 2 | the number of the command and its fields (below) |
| `KindConfig` | 3 | the version of the config |
| `KindHash` | 4 | the hash, 8 bytes |
| `KindStableRead` | 5 | state ID of the engine read, the tick of the copy |
| `KindReturned` | 6 | the header of a message the engine sent that came back (state ID of the sender, `Seq`), the reason `u8` (`sim.Reason`) |

- Integers are `uvarint` unless a width is said. The address of an engine is its state ID.
- An inbound message is recorded whole.
- A returned message is recorded at the tick its sender settled it: the tick of the send when the router refuses it at once, the tick it is taken from the return queue otherwise.
- A receiver drops a message whose `Seq` is not larger than the last it took from the same sender, and records nothing for it. `Seq` 0 is never a copy.

## Messages and actions

A message, in order:

| Field | Written as |
|---|---|
| destination | the state ID of the engine; its type says which actions the message carries |
| header | state ID of the sender, `Seq` |
| attempts | `uvarint` |
| `OnFail` | below |
| actions | count, then each action |

An action is its ID, then its fields written by its `Encode`; its decoder is registered with `sim.Codecs` (`RegisterAction[S]`), found by the ID, and an action that is not an action of the type of state of its message is an error. Private fields of an action that waits are written.

`OnFail` is its ID (`uvarint`) and then:

| Kind | Body |
|---|---|
| `onfail.Retry`, `onfail.Drop` | nothing |
| `onfail.Redirect`, `onfail.Escalate` | a state ID |
| `onfail.Compensate` | a message, written as above, at most 8 deep |
| `onfail.Notify` | key string, count, each argument string |

- A codec refuses a number or a Go type that two kinds register, and a test fails when an action ID, a command ID or an `OnFail` ID of a register has no decoder. Writing needs no register: an action, a command or an `OnFail` writes its own ID.
- `EncodeMessage` returns `ErrNoCodec`, and writes nothing, for a message that holds an action with no `Encode`, or a nil `OnFail`; and `ErrBadValue`, and writes nothing, for messages nested more than 8 deep, which is the most that can be read back.

## Meta of a segment

| Field | Value |
|---|---|
| data version | `uvarint`, 1: given to every `Decode` as a `codec.Version` |
| engine | state ID, whose type is the type of its state |
| seed | 8 bytes |
| first tick | tick of the first record of the segment |

- The meta is made by the goroutine of the writer from the first record of the segment. `replay.ReadFileLog` starts at the last segment whose first tick is not after the tick asked for.
- A reader refuses a segment of another data version (in pre-alpha there is one) and a log of another engine.

## Numbers

Each of these is part of the format of the log: it never changes, is never used for another thing, and a new thing takes the next number. An action ID, a cooldown key and a command ID are numbers themselves (the constants of their registers), written as they are; a command writes its own fields with its method `Encode`, and a function of its file reads them, found in a table by the ID.

| What | Table | Numbers |
|---|---|---|
| state type | `server/game/sim/state/id.go` (the value of the constant; the high 8 bits of a state ID) | 1 to 3 |
| action ID | `server/game/sim/action/id.go` (the value of the constant) | 1 to 15 |
| `OnFail` ID | `platform/sim/onfail/onfail.go` (the value of the constant) | 1 to 6 |
| cooldown key | `server/game/sim/action/cooldown_key.go` (the value of the constant) | 1, 2 |
| command ID | `server/game/sim/command/id.go` (the value of the constant) | 1 to 9 |

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

## Running it

- `go run ./server/cmd/world -replay-dir <dir>` runs the script of `server/cmd/world` with a `File` as the replay log of the cluster, closes it at the end, and reads it back to say what each engine recorded.
- `TestAProcessThatIsKilledLeavesALogThatReadsAndCanBeContinued` starts a process that records on three engines as fast as it can, kills it with SIGKILL at six different moments in the same directory, and requires that each log reads, that every tick in it has all its records, and that the next process goes on from the end of the log. A kill can stop a write between two pages of the kernel, which left a tick of 43 records of 100 in the first version of the log; a batch is now all or nothing ([segmentlog](segmentlog.md)), and the tick that was being written is simply not in the log. What a crash of the machine can leave is tested by cutting the end of a file at every byte.

## Not here yet

- Checkpoints and the scheduler in a checkpoint. The replay from tick 0 is in [replay_runner](replay_runner.md).
- A `Close` is needed to write what the last ticks recorded: until then they are in memory and on the way to disk.

Benchmarks: [replay_log](benchmark/replay_log.md).
