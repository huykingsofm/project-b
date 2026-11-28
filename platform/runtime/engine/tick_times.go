package engine

import (
	"fmt"
	"math/bits"
	"strings"
	"time"
)

// tickTimeClasses is how many classes of the time of a tick are counted: the class i holds the
// ticks that took less than 2^i microseconds and not less than half of that, and the last holds
// the ticks that took as long as the one before it or longer (65 ms and more).
const tickTimeClasses = 18

// tickTimes counts how long the ticks of an engine took, in classes of a power of two of
// microseconds, and the longest. It is for the log: the engine reads the clock to measure a tick
// and to nothing else, so what the engine does does not depend on it.
type tickTimes struct {
	counts  [tickTimeClasses]uint64
	ticks   uint64
	longest time.Duration
}

// add counts a tick that took the time given.
func (h *tickTimes) add(d time.Duration) {
	h.counts[min(bits.Len64(uint64(d.Microseconds())), tickTimeClasses-1)]++
	h.ticks++
	h.longest = max(h.longest, d)
}

// reset forgets what was counted.
func (h *tickTimes) reset() { *h = tickTimes{} }

// atMost is the time that p of the ticks (from 0 to 1) took at most, as the upper bound of the
// class that holds them.
func (h *tickTimes) atMost(p float64) time.Duration {
	want := uint64(float64(h.ticks)*p + 0.5)
	var seen uint64
	for class, n := range h.counts {
		seen += n
		if seen >= want && seen > 0 {
			return time.Duration(1<<class) * time.Microsecond
		}
	}
	return h.longest
}

// text is the classes that hold a tick: the upper bound of each in microseconds, and how many.
func (h *tickTimes) text() string {
	var parts []string
	for class, n := range h.counts {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("<%dus=%d", 1<<class, n))
		}
	}
	return strings.Join(parts, " ")
}
