# R31. No panic while the server runs (hard)

Once the server has started, no code path panics. A mistake that is found while running is returned as an `error` to a caller that can log or answer it, and when nothing above can take an error, it is logged with the `logging` package (`logging.Warn` or `logging.Error`, [docs](../../docs/logging.md)) and the code carries on with the rest of its work. A fault that is handled by dropping or skipping something is never silent: it is logged.

- **Allowed:** a `panic` at startup, for content or configuration that is wrong and that the owner must fix before the server can run (a config variable declared twice); a map is returned as an error, because maps will be read from files. It happens before the first tick.
- **Not allowed:** a `panic` reached by a message, a command, a tick or data read at runtime; an index, a nil or a division that data from outside can reach without a check.
- **Type assertions** on a value the package itself controls need no check, because that value cannot have another type.
- **What the log says:** the failing thing as key and value pairs (`zone=1 layer=2 err=...`), not a sentence to be parsed.
- A bad message must not stop an engine: the engine drops it, logs it and takes the next one (see also [R19](r19-actions-do-not-fail.md)).
