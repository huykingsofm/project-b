package logging

import (
	"sync/atomic"
	"time"
)

// queue is the channel between the loggers and the goroutine of Init.
type queue struct {
	entries chan Entry
	dropped atomic.Uint64
}

// queueLogger puts an entry into the channel of its queue; when the channel is full it counts the
// entry and drops it, because a log call must never hold a tick.
type queueLogger struct {
	q      *queue
	fields []any
}

func (l *queueLogger) Debug(msg string, kv ...any) { l.log(LevelDebug, msg, kv) }
func (l *queueLogger) Info(msg string, kv ...any)  { l.log(LevelInfo, msg, kv) }
func (l *queueLogger) Warn(msg string, kv ...any)  { l.log(LevelWarn, msg, kv) }
func (l *queueLogger) Error(msg string, kv ...any) { l.log(LevelError, msg, kv) }

func (l *queueLogger) With(kv ...any) Logger {
	return &queueLogger{q: l.q, fields: appendFields(l.fields, kv)}
}

func (l *queueLogger) log(level Level, msg string, kv []any) {
	select {
	case l.q.entries <- Entry{Time: time.Now(), Level: level, Message: msg, Fields: appendFields(l.fields, kv)}:
	default:
		l.q.dropped.Add(1)
	}
}
