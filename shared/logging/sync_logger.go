package logging

import (
	"fmt"
	"os"
	"time"
)

// syncLogger gives every entry to its consumer at once, in the goroutine of the caller. It is the
// logger of a program that has not called Init, where a line must not be lost and nothing is
// flushing.
type syncLogger struct {
	c      Consumer
	fields []any
}

func newSyncLogger(c Consumer) *syncLogger { return &syncLogger{c: c} }

func (l *syncLogger) Debug(msg string, kv ...any) { l.log(LevelDebug, msg, kv) }

func (l *syncLogger) Info(msg string, kv ...any) { l.log(LevelInfo, msg, kv) }

func (l *syncLogger) Warn(msg string, kv ...any) { l.log(LevelWarn, msg, kv) }

func (l *syncLogger) Error(msg string, kv ...any) { l.log(LevelError, msg, kv) }

func (l *syncLogger) With(kv ...any) Logger {
	return &syncLogger{c: l.c, fields: appendFields(l.fields, kv)}
}

func (l *syncLogger) log(level Level, msg string, kv []any) {
	e := Entry{Time: time.Now(), Level: level, Message: msg, Fields: appendFields(l.fields, kv)}
	if err := l.c.Consume([]Entry{e}); err != nil {
		fmt.Fprintf(os.Stderr, "logging: the consumer failed and 1 entry is lost: %v\n", err)
	}
}

// defaultLogger is the global logger when no log is running.
func defaultLogger() Logger { return newSyncLogger(NewWriterConsumer(os.Stderr)) }
