// Package logging writes log lines as key=value pairs. A call only puts an entry into a channel and
// returns: it never encodes, never writes and never waits. A Consumer, given to Init, takes the entries
// out on a timer, so a log call costs the caller one channel send. It imports only the channels package.
//
// The package keeps one global Logger. Before Init and after the function Init returned has run, it is a
// synchronous logger that writes each line to stderr at once, so a line is never lost and code that logs
// never needs a check. The functions Debug, Info, Warn and Error
// write through it; With makes a logger of its own that carries fixed fields.
package logging

import (
	"sync/atomic"
	"time"
)

// Logger writes log lines. A value in a field is read later, when the Consumer encodes it, so it must
// not change after the call: pass numbers, strings, errors and values, not a pointer to state that is
// still changing.
type Logger interface {
	// Debug, Info, Warn and Error log msg at their level, followed by the pairs kv (key, value, key, ...).
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
	// With returns a logger whose lines start with the pairs kv; the receiver is unchanged.
	With(kv ...any) Logger
}

// Entry is one log call as it was made. Fields holds the key and value pairs in order.
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Fields  []any
}

// global is the logger of the package functions.
var global atomic.Pointer[Logger]

func init() { setGlobal(defaultLogger()) }

func setGlobal(l Logger) { global.Store(&l) }

// Global returns the logger of the package functions: a synchronous one until Init is called.
func Global() Logger { return *global.Load() }

// Debug logs at LevelDebug through the global logger.
func Debug(msg string, kv ...any) { Global().Debug(msg, kv...) }

// Info logs at LevelInfo through the global logger.
func Info(msg string, kv ...any) { Global().Info(msg, kv...) }

// Warn logs at LevelWarn through the global logger.
func Warn(msg string, kv ...any) { Global().Warn(msg, kv...) }

// Error logs at LevelError through the global logger.
func Error(msg string, kv ...any) { Global().Error(msg, kv...) }

// With returns a logger of the global logger that starts its lines with the pairs kv. It keeps writing
// where it was made from: a logger made after Init puts its entries in the queue, and after close the
// queue is no longer read.
func With(kv ...any) Logger { return Global().With(kv...) }

// appendFields returns the fixed fields followed by kv, in a slice of its own.
func appendFields(fixed, kv []any) []any {
	if len(fixed) == 0 {
		return kv
	}
	out := make([]any, 0, len(fixed)+len(kv))
	out = append(out, fixed...)
	return append(out, kv...)
}
