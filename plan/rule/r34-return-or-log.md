# R34. An error is returned or logged, never both (hard)

A function that meets an error does one of two things with it: it returns it, or it logs it. It never does both. The error is logged once, by the one that handles it last: the caller that can do nothing more than say it, such as a cluster that stops, a loop that drops a message and goes on, or `main`.

- **Returned:** the error is wrapped with where it went through, `fmt.Errorf("the engine %d: %w", id, err)`, so that the one that logs it says, in one line, the path from where it started. A sentinel (`var ErrX = errors.New(...)`) is wrapped too, and found with `errors.Is`.
- **Logged:** with the `logging` package and the failing thing as key and value pairs ([R31](r31-no-panic-at-runtime.md)), and then the function carries on or stops; it does not return the error as well.
- **Not allowed:** logging an error and then returning it, or returning it to a caller that logs it again, which writes the same fault twice and makes a reader think there were two.

Why: one fault is one line in the log, and the wrapping says where it came from, so the log is neither noisy nor missing what it needs.
