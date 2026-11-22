package logging

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

// ErrAlreadyInitialized is returned by Init while an earlier Init has not been closed.
var ErrAlreadyInitialized = errors.New("logging: already initialized")

// ErrNoConsumer is returned by Init when the consumer is nil.
var ErrNoConsumer = errors.New("logging: a consumer is needed")

type options struct {
	capacity   int
	flushEvery time.Duration
}

// Option changes how Init sets the log up.
type Option func(*options)

// WithCapacity sets how many entries wait for the next flush (4096 by default); a value below one
// is ignored. A full queue drops the entry and the next flush says how many were dropped.
func WithCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.capacity = n
		}
	}
}

// WithFlushEvery sets how often the consumer is given the entries (one second by default); a value
// that is not positive is ignored.
func WithFlushEvery(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.flushEvery = d
		}
	}
}

var (
	initMu      sync.Mutex
	initialized bool
)

// Init makes the global logger and starts a goroutine that gives the entries to c every flush
// period (one second unless WithFlushEvery says otherwise). It returns the function that stops the
// log: it makes the global logger synchronous again, gives c what is left and waits for it. Call it
// once, at the start of main, and defer the function:
//
//	closeLogs, err := logging.Init(consumer)
//	if err != nil { ... }
//	defer closeLogs()
//
// Init returns an error when a log is already running or when c is nil, and then changes nothing.
func Init(c Consumer, opts ...Option) (func(), error) {
	if c == nil {
		return nil, ErrNoConsumer
	}
	o := options{capacity: 4096, flushEvery: time.Second}
	for _, opt := range opts {
		opt(&o)
	}
	initMu.Lock()
	defer initMu.Unlock()
	if initialized {
		return nil, ErrAlreadyInitialized
	}
	initialized = true

	q := &queue{entries: make(chan Entry, o.capacity)}
	root := &queueLogger{q: q}
	setGlobal(root)
	stop, done := make(chan struct{}), make(chan struct{})
	go q.run(c, o.flushEvery, stop, done)

	var once sync.Once
	return func() {
		once.Do(func() {
			initMu.Lock()
			defer initMu.Unlock()
			setGlobal(defaultLogger())
			close(stop)
			<-done
			initialized = false
		})
	}, nil
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
