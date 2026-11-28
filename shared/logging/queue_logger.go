package logging

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/huykingsofm/project-b/shared/channels"
)

// queueLogger puts an entry into the channel of its queue; when the channel is full it counts the
// entry and drops it, because a log call must never hold a tick.
type queueLogger struct {
	q      *queue
	fields []any
}

func (l *queueLogger) Debug(msg string, kv ...any) { l.log(LevelDebug, msg, kv) }

func (l *queueLogger) Info(msg string, kv ...any) { l.log(LevelInfo, msg, kv) }

func (l *queueLogger) Warn(msg string, kv ...any) { l.log(LevelWarn, msg, kv) }

func (l *queueLogger) Error(msg string, kv ...any) { l.log(LevelError, msg, kv) }

func (l *queueLogger) With(kv ...any) Logger {
	return &queueLogger{q: l.q, fields: appendFields(l.fields, kv)}
}

func (l *queueLogger) log(level Level, msg string, kv []any) {
	select {
	case l.q.entries <- Entry{
		Time: time.Now(), Level: level, Message: msg, Fields: appendFields(l.fields, kv),
	}:
	default:
		l.q.dropped.Add(1)
	}
}

// queue is the channel between the loggers and the goroutine of Init.
type queue struct {
	entries chan Entry
	dropped atomic.Uint64
}

// run gives the entries to c every period, and once more when it is told to stop.
func (q *queue) run(c Consumer, every time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			q.flush(c)
		case <-stop:
			q.flush(c)
			return
		}
	}
}

// flush gives c the entries that are in the channel now. When entries were dropped since the last
// flush, the batch starts with a warn entry that says how many.
func (q *queue) flush(c Consumer) {
	entries := channels.Drain(q.entries)
	if dropped := q.dropped.Swap(0); dropped > 0 {
		entries = append(
			[]Entry{
				{
					Time:    time.Now(),
					Level:   LevelWarn,
					Message: "log entries dropped",
					Fields:  []any{"dropped", dropped},
				},
			},
			entries...,
		)
	}
	if len(entries) == 0 {
		return
	}
	if err := c.Consume(entries); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"logging: the consumer failed and %d entries are lost: %v\n",
			len(entries),
			err,
		)
	}
}
