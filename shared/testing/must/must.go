// Package must is for tests: it turns a call that reports whether it worked into a call that
// stops the test run when it did not.
package must

// Ok returns v when ok, and panics when not: a test that reaches it did not set up what it needs.
func Ok[T any](v T, ok bool) T {
	if !ok {
		panic("must.Ok: the call did not work")
	}
	return v
}
