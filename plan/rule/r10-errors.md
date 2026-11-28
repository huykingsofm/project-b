# R10. Errors: actions do not fail, nothing panics, an error is returned or logged once (hard)

## Actions do not fail

An action returns no error: when what it was asked for is not possible it changes nothing, or says so through the state (a refusal), and that is its result. What must happen outside the engine (a message to another engine, an action for itself at a later tick) goes in the registry, and the engine does it when the plan has run. Every send names its `OnFail` (what a failed delivery means); a send that makes no choice does not compile.

## No panic while the server runs

Once the server has started, no code path panics. A mistake that is found while running is returned as an `error` to a caller that can log or answer it, and when nothing above can take an error, it is logged with the `logging` package (`logging.Warn` or `logging.Error`, [docs](../../docs/logging.md)) and the code carries on with the rest of its work. A fault that is handled by dropping or skipping something is never silent: it is logged.

- **Allowed:** a `panic` at startup, for content or configuration that is wrong and that the owner must fix before the server can run (a config variable declared twice); a map is returned as an error, because maps will be read from files. It happens before the first tick.
- **Not allowed:** a `panic` reached by a message, a command, a tick or data read at runtime; an index, a nil or a division that data from outside can reach without a check.
- **Type assertions** on a value the package itself controls need no check, because that value cannot have another type.
- **What the log says:** the failing thing as key and value pairs (`zone=1 layer=2 err=...`), not a sentence to be parsed.
- A bad message must not stop an engine: the engine drops it, logs it and takes the next one.

## Returned or logged, never both

A function that meets an error does one of two things with it: it returns it, or it logs it. It never does both. The error is logged once, by the one that handles it last: the caller that can do nothing more than say it, such as a cluster that stops, a loop that drops a message and goes on, or `main`.

- **Returned:** the error is wrapped with where it went through, `fmt.Errorf("the engine %d: %w", id, err)`, so that the one that logs it says, in one line, the path from where it started. A sentinel (`var ErrX = errors.New(...)`) is wrapped too, and found with `errors.Is`.
- **Logged:** with the `logging` package and the failing thing as key and value pairs (as under "No panic" above), and then the function carries on or stops; it does not return the error as well.
- **Not allowed:** logging an error and then returning it, or returning it to a caller that logs it again, which writes the same fault twice and makes a reader think there were two.

Why: one fault is one line in the log, and the wrapping says where it came from, so the log is neither noisy nor missing what it needs.
