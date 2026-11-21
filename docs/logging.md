# Logging

`shared/logging`. Log lines as `key=value` pairs. A call puts an entry into a channel and returns; a goroutine that `Init` starts gives the entries to a `Consumer` every flush period. The package imports only `shared/channels`. The capacity and the flush period are not config: the caller decides them with `WithCapacity` and `WithFlushEvery`, and the package uses 4096 and one second when it is not told.

## Use

```go
closeLogs, err := logging.Init(logging.NewWriterConsumer(os.Stderr))
if err != nil { ... }
defer closeLogs()

logging.Warn("returned message lost", "from", from, "reason", r)
l := logging.With("zone", 1) // a Logger that starts its lines with zone=1
l.Error("character does not enter", "err", err)
```

## Line

```
time=2026-11-21T10:00:00.123456Z level=warn msg="config snapshot mismatch" var=zonelayer.pickup.radius tick=1042
```

| Field | Value |
|---|---|
| `time` | UTC, microseconds, taken at the call |
| `level` | `debug`, `info`, `warn`, `error` |
| `msg` | the message |
| the rest | the pairs of `With` first, then the pairs of the call, in order |

## Values

| Value | Written as |
|---|---|
| string | as is; quoted when empty or holding a space, `=`, `"` or a control character |
| `error` | its `Error()`, as a string |
| integers, floats, `bool` | the number or `true` / `false` |
| `time.Duration` | `2s` |
| `time.Time` | UTC, same layout as `time` |
| `nil` | `null` |
| anything else | `fmt.Sprint`, as a string |

- A key that is not a string is written `!BADKEY:<key>`; an empty key `!EMPTYKEY`; a key with a space, `=` or `"` has them replaced by `_`.
- A key without a value is written `key=!MISSING`.
- A value is read when the consumer encodes it, so it must not change after the call.

## Interfaces

| Interface | Methods |
|---|---|
| `Logger` | `Debug`, `Info`, `Warn`, `Error(msg, kv...)`; `With(kv...) Logger`, which returns a new logger and leaves the receiver unchanged |
| `Consumer` | `Consume(entries []Entry) error`, called by one goroutine at a time with a batch in the order the entries were made |

## Package

| Name | Result |
|---|---|
| `Init(c, opts...)` | makes the global logger, starts the flush goroutine and returns `(close func(), err error)` |
| `Debug`, `Info`, `Warn`, `Error` | log through the global logger |
| `With(kv...)` | `Global().With(kv...)` |
| `Global()` | the global logger |
| `NewWriterConsumer(w)` | a `Consumer` that encodes a batch as lines and writes it with one `Write` |
| `WithCapacity(n)` | entries that wait for a flush; default 4096 |
| `WithFlushEvery(d)` | flush period; default 1 second |

## Behavior

- Before `Init` and after `close` the global logger is synchronous: each call is encoded and written to stderr at once, in the goroutine of the caller, so no line is lost. A logger made by `With` while the log runs writes to its queue, and after `close` nobody reads that queue.
- `Init` returns `ErrNoConsumer` for a nil consumer and `ErrAlreadyInitialized` while an earlier `Init` is not closed; it changes nothing then.
- `close` makes the global logger drop, flushes what is left and waits for the goroutine; calling it again does nothing.
- A full queue drops the entry and counts it; the next batch starts with `level=warn msg="log entries dropped" dropped=N`.
- A `Consume` that returns an error loses its batch and is reported on stderr; the log goes on.
- No level filter: every call is kept.

Benchmarks: [logging](benchmark/logging.md).
