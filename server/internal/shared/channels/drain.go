// Package channels holds small helpers for Go channels. It imports nothing of the project.
package channels

// Drain takes what is in a channel when it is called, and no more: what arrives while the tick runs
// waits for the next tick, so a flood cannot hold a tick.
func Drain[C any](ch <-chan C) []C {
	n := len(ch)
	out := make([]C, 0, n)
	for ; n > 0; n-- {
		select {
		case c := <-ch:
			out = append(out, c)
		default:
			return out
		}
	}
	return out
}
