package config

import "sync/atomic"

// Live is the configuration that the configuration engine published last, for whoever is outside
// the engines and cannot read it through the stable view of one: a writer of files, a service.
// It is changed as the engine publishes, so it follows the configuration in real time; what the
// engines themselves read goes through their stable view, which the event log records. It is safe
// for any goroutine, and a nil *Live gives the defaults.
type Live struct {
	snap atomic.Pointer[ReadOnlySnapshot]
}

// NewLive makes a Live that holds the defaults.
func NewLive() *Live { return &Live{} }

// Set makes the snapshot the configuration that Get returns.
func (l *Live) Set(s ReadOnlySnapshot) { l.snap.Store(&s) }

// Get returns the configuration that was set last, or the defaults when none was.
func (l *Live) Get() ReadOnlySnapshot {
	if l != nil {
		if p := l.snap.Load(); p != nil {
			return *p
		}
	}
	return Defaults()
}
