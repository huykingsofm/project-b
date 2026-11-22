# Benchmarks of the codecs of actions and messages

Conventions and how to run: [index](index.md). Behavior: [replay_log](../replay_log.md#messages-and-actions).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkEncodeMessage/<message>` | `sim/action` | `EncodeMessage` into a reused buffer: the part of recording a message done by the goroutine of the engine |
| `BenchmarkDecodeMessage/<message>` | `sim/action` | `DecodeMessage` from its bytes |

## Results

| Message | Encode ns/op | B/op | allocs/op | Decode ns/op | B/op | allocs/op |
|---|---|---|---|---|---|---|
| 2 actions (a move and a chat line) | 84.7 | 0 | 0 | 321 | 293 | 7 |
| enter, a character with a cooldown | 83.1 | 0 | 0 | 468 | 536 | 9 |
| a world message with an `OnFailCompensate` that holds a message | 131.9 | 0 | 0 | 466 | 440 | 7 |

- Encoding allocates nothing; decoding allocates the message, its actions and their strings.
- The cost grows with the actions and with the messages inside `OnFail`, not with a table: the decoder of an action is found by its ID in a map.
