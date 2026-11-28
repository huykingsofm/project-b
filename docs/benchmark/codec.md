# Benchmarks of the codecs of actions and messages

Conventions and how to run: [index](index.md). Behavior: [eventlog](../eventlog.md#messages-and-actions).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkEncodeMessage/<message>` | `server/game/sim/action` | `EncodeMessage` into a reused buffer: the part of recording a message done by the goroutine of the engine |
| `BenchmarkDecodeMessage/<message>` | `server/game/sim/action` | `DecodeMessage` from its bytes |

## Results

| Message | Encode ns/op | B/op | allocs/op | Decode ns/op | B/op | allocs/op |
|---|---|---|---|---|---|---|
| 2 actions (a move and a chat line) | 51 | 0 | 0 | 269 | 237 | 7 |
| enter, a character with a cooldown | 68 | 0 | 0 | 465 | 496 | 10 |
| a world message with an `onfail.Compensate` that holds a message | 97 | 0 | 0 | 439 | 376 | 10 |

- Encoding allocates nothing; decoding allocates the message, its actions and their strings.
- An action is a value type: its decoder returns the value, which is put in the interface of the action, and its methods, `Encode` among them, are called on the value.
- The cost grows with the actions and with the messages inside `OnFail`, not with a table: the decoder of an action is found by its ID in a map.
