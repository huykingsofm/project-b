# Replay log in files

`platform/replay/` (`log.go`, `record.go`, `mem.go`, `file.go`, `reader.go`). The `replay.Log` that writes to files, and the reader of those files. What is recorded and why: [simulation](simulation.md#replay). The storage under it: [segmentlog](segmentlog.md).

## Files

| Part | Where |
|---|---|
| a log for each engine | `<dir>/<name>-<n>/`: the name of the type of its state (`StateType.String()`, see below) and the number of the state among the states of its type, for example `world-1`, `zone-2`, `zonelayer-1`; the meta of the segments holds the whole ID |
| the segments of a log | `seg-<index>.log` ([segmentlog](segmentlog.md#files)) |
| opened | by the first tick of the engine: every tick has a frame, an empty one when the engine recorded nothing |

- An engine writes only to its own log, with no lock shared with the others. Its methods are called from the goroutine of the engine.
- A log writes what it is given: `NopLog` is a place holder that takes everything and keeps nothing, and the engine works out a hash at its period whatever the log is.
- The name of a type of state is set where the type is defined, with `sim.MapStateTypeToName(type, name)`: a type has one name, a name belongs to one type, and neither is ever changed, because the name is in the path of a log on the disk. A second name for a type, or a name that another type has, is an error that is found when the program starts. A type that was given none is called `state type <number>`.

## Writing

| Method of `replay.Log` | What it does |
|---|---|
| `Append(engine, tick, body)` | records what the engine took, sent or read at the tick: it is encoded into the buffer of the tick |
| `EndTick(engine, tick)` | says that the engine has recorded all it will for the tick, the hash of its state included when it is due; a log in files hands the frame of the tick to the goroutine that writes it |

- The engine writes the hash as the last record of the tick, in `EndTick`, every `every` ticks (the period and the offset of each engine). A tick is whole when its frame is: a frame is one record of the segment log, written whole or not at all.
- The tick of a frame is the one after the last of the log, and the first of a log is the tick 1 (the state of a new engine is at the tick 0): a log has no gap, and a frame that does not follow is refused and the log marked as failed. `Log.Last(engine)` is the tick of the last frame, written or not, and 0 for a log with none.
- `Engine.Remove` (`Cluster.Remove`) ends an engine for good: its log gets a last frame, of the tick after the last it ran, that holds `Retired`; what it published is forgotten, its queue goes back to its senders, and a message to it bounces as `DeliveryFailureRetired`. Stopping an engine with the context of `Run` is not removing it: nothing is written, and it goes on after a restart.

## Which tick is on the disk

| Call | Answer |
|---|---|
| `FileLog.Durable(engine)` | the tick of the last frame that is on the disk, so a crash loses nothing at or before it; the last tick of the log that was there when it was opened counts; 0 for an engine with no log |

- The tick of a frame is its ID in the segment log, so `Synced` (the ID of the last record on the disk) is the answer as it is.
- The answer is a lower bound: it is behind the tick that was last written by up to what one `SyncEvery` holds, and it never goes back. It reads an atomic, and is safe to call from any goroutine, an engine that does not tick any more included.

## Which tick is safe

| Call | Answer |
|---|---|
| `Log.Safe(engine)` | the tick of the engine up to which a crash takes back nothing it did and nothing it took |
| `FileLog.Safe(engine)` | it is `Durable`, but before the first tick that depends on a tick of another engine that is not safe: a message that the engine took, or a copy of another engine that it read, says what that engine did |
| `NopLog`, `MemLog`, the log of a replay | every tick: a log that records nothing cannot make a tick more safe |

- What a tick depends on is made by its records (`Record.Dependency()`): the log of each engine keeps it, tick by tick, for the ticks that are not safe yet. A goroutine of the `FileLog` works out again what is safe for every engine each time a log has synced (`segmentlog.Options.OnSync`), until none changes, and `Close` does it once more when every log is on the disk.
- A tick that depends on an engine whose log is not opened yet waits for it, and a tick whose dependencies are all safe stays safe, so a dependency is looked at once.
- An engine whose log was there before it was opened is safe up to the last tick of that log: what is in a log is what a restart goes on from.

## Recovery line

| Call | What it does |
|---|---|
| `replay.RecoveryLine(dir, codecs)` | the last tick of each log under `dir` that a recovery keeps |
| `replay.CutToRecoveryLine(dir, codecs)` | cuts each log to that tick (`segmentlog.CutAfter`, so what is cut is kept next to the segments) and returns the ticks; no log may be open for writing |

- An engine starts at the last tick of its log and is cut before the first tick that depends (`Record.Dependency()`) on a tick of another engine that is cut away, or of an engine that has no log (its limit is 0). This is repeated until nothing changes, since an engine that was cut may be what another depends on. A tick is cut whole, with the commands of the clients that it took, and with everything after it.
- What is left is a state of the cluster in which no engine took a message that its sender did not send, or read a copy that its source did not publish. Each engine can be made again from its own log, with no other: the logs that are left replay with no divergence (`runner.Run`), which a test checks on a cluster whose first zonelayer lost its tail.
- It is the same rule as `Safe`, from the other side: `Safe` is what the line is while the cluster runs, and the line is what `Safe` was when it stopped, or less.
- Not here yet: checkpoint files, so that an engine is made from one and run to its limit instead of from tick 0, and the engines that go on live after it ([roadmap](../plan/en/roadmap.md)).

## Records

What an engine writes in a tick is one frame, which is one record of the segment log and so one batch of it:

| Part | Layout |
|---|---|
| frame (the payload of a segment log record) | the records of the tick, in the order they were appended; its tick is the ID of the segment log record |
| record | `[length uvarint][kind u8][body]`, the length being that of the kind and the body |

| Where | What it holds |
|---|---|
| the tick of a frame | the ID of its record in the segment log: the begin in the header of each segment says the tick of its first frame, so no frame and no record carries the tick |
| a tick with no record | a frame with no record at all, so the batches of a log are its ticks one by one, and the number of batches that are on the disk (`segmentlog.Writer.Synced`) says up to which tick nothing is lost |
| the engine | not in a record: it is the engine of the log |
| the meta of a segment | the format (1), the engine and the seed |

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
| `KindReturned` | 6 | the header of a message the engine sent that came back (state ID of the sender, `Seq`, the tick it was sent at), the reason `u8` (`sim.DeliveryFailure`) |
| `KindRetired` | 7 | nothing: the engine was removed for good; it is the only record of the last frame of its log |

- Integers are `uvarint` unless a width is said. The address of an engine is its state ID.
- An inbound message is recorded whole.
- A returned message is recorded at the tick its sender settled it: the tick of the send when the router refuses it at once, the tick it is taken from the return queue otherwise.
- A message carries the tick of its sender when it was sent (`Header.Tick`, set when the engine sends it). An `Inbound` record makes the engine depend on that tick of the sender, and a `StableRead` record, which has the tick of the copy it read, makes it depend on that tick of the source: `Record.Dependency()` says which as a `sim.StateDependency{Source, Tick}`, or false when a record depends on nothing (a message with no tick, a read that found no copy, any other kind).
- A receiver drops a message whose `Seq` is not larger than the last it took from the same sender, and records nothing for it. `Seq` 0 is never a copy.

## Messages and actions

A message, in order:

| Field | Written as |
|---|---|
| destination | the state ID of the engine; its type says which actions the message carries |
| header | state ID of the sender, `Seq`, the tick of the sender at which it sent the message |
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
