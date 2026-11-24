# Outbox and acks

`platform/engine/outbox.go`, `platform/engine/cell.go` (`flush`, `publishAcks`), `platform/replay/safe.go`. What an engine keeps of the messages it sends until the engine that took them has them where a crash does not take them back. The design: `plan/en/roadmap.md`. Not to be mistaken for `sim.Outbox`, the list that the actions of a tick put the messages in, which is emptied when the tick sends them.

## What the outbox holds

| Part | What it is | When it goes |
|---|---|---|
| unsent | a message that its `OnFail` sends again (`Resend`), and that did not go yet | at the next flush, with a new number |
| sent | a message that went into the channel of its receiver, with the number and the tick it was stamped with | when its receiver acks it |

- Both are in the checkpoint, each message whole. What a sent message waits for (the tick at which it is sent again) is not: after a restore each is sent again at the next flush, with the number it has.
- The hash of the engine covers the unsent messages and not the sent ones: when a message is acked depends on when the log of its receiver is synced, which a replay does not repeat.
- An engine in an environment with no `AckBoard` (a replay) holds no sent message: nothing would ever let it go.

## Sending again

| When | What the engine does |
|---|---|
| a message went and is not acked after `outbox.resend_first_ticks` (20) ticks of its sender | sends it again, the same message with the same number and tick; then after the wait grows by `outbox.resend_step_ticks` (20), up to `outbox.resend_most_ticks` (200) |
| the router says the receiver is full or not started | the message waits again |
| the router says the receiver is gone (`DeliveryFailureRetired`, `DeliveryFailureNotGiven`) | the message is let go of and comes back to the engine as a message that could not be delivered (`Router.Return`), so its `OnFail` runs once, at the tick that the replay log records |

- A receiver drops a message whose number is not more than the last it took from the sender, and records nothing of it. So a second copy of a message costs a slot in the channel, nothing else, and a message that is sent again after a restore is taken once.
- A message that its `OnFail` sends again gets a new number: it was never taken, and the same number would be dropped as a copy once a later message of the sender was taken.

## Acks

| Part | What it is |
|---|---|
| `AckBoard` | `Slot(receiver, sender)`: a number that only goes up, in the memory of the process; it is not a message, and is never written to a log or a checkpoint |
| receiver | at the end of each tick asks its replay log up to which tick it is safe (`Log.Safe`), and for the messages it took in those ticks sets the slot of each sender to the largest number it took |
| sender | at each flush reads the slot of each receiver it holds a message for, and lets go of the messages for it that have a number up to it |
| a restore | what the checkpoint says an engine took (`lastSeq`) is in a place that no crash takes back, so it is set in the slots at once |

- A copy that a receiver drops is not acked by being dropped: it has to be safe like any other message.
- Safe: see [replay_log](replay_log.md#which-tick-is-safe).
