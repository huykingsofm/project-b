# Unsettled messages and acks

`platform/engine/unsettled_messages.go`, `platform/engine/cell.go` (`flushUnsettledMessages`, `releaseAckedMessages`, `publishAcks`), `platform/replay/safe.go`. The unsettled messages of an engine (`unsettledMessages`) are what it keeps of the messages it sends until the engine that took them has them where a crash does not take them back. The design: `plan/en/roadmap.md`. They are not `sim.Outbox`, the list that the actions of a tick put the messages in, which is emptied when the tick sends them.

## What they hold

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
| a message went and is not acked after `unsettled_messages.resend_first_ticks` (20) ticks of its sender | sends it again, the same message with the same number and tick; then after the wait grows by `unsettled_messages.resend_step_ticks` (20), up to `unsettled_messages.resend_most_ticks` (200) |
| the router says the receiver is full or not started | the message waits again |
| the router says the receiver is gone (`DeliveryFailureRetired`) | the message is let go of and comes back to the engine as a message that could not be delivered (`Router.Return`), so its `OnFail` runs once, at the tick that the replay log records |

- A receiver drops a message whose number is not more than the last it took from the sender, and records nothing of it. So a second copy of a message costs a slot in the channel, nothing else, and a message that is sent again after a restore is taken once.
- A message that its `OnFail` sends again gets a new number: it was never taken, and the same number would be dropped as a copy once a later message of the sender was taken.

## Acks

| Part | What it is |
|---|---|
| `AckBoard` | a number for each pair (receiver, sender) that only goes up, in the memory of the process: `Raise(receiver, sender, seq)` says it and `Acked(receiver, sender)` reads it (0 when nothing was said); it is not a message, and is never written to a log or a checkpoint |
| receiver | at the end of each tick asks its replay log up to which tick it is safe (`Log.Safe`), and for the messages it took in those ticks raises the number of each sender to the largest it took (`publishAcks`) |
| sender | at each flush (`releaseAckedMessages`) reads the number of each receiver it holds a message for, and lets go of the messages for it that have a number up to it |
| a restore | what the checkpoint says an engine took (`lastRecvSeq`) is in a place that no crash takes back, so it is raised on the board at once |

- A copy that a receiver drops is not acked by being dropped: it has to be safe like any other message.
- Safe: see [replay_log](replay_log.md#which-tick-is-safe).
