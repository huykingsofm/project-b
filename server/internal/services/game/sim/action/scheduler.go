package action

// Scheduler keeps what an engine runs at a later tick: the actions that were set for one tick. What
// must happen again and again is an action that sets itself again when it runs (R7), so there is
// nothing else for the scheduler to keep: all that the engine will do on its own waits here, or in
// what it set for the next tick. The tick count it knows is the last one it gave out, so an action
// set for a tick that has passed runs at the next one.
type Scheduler[A Action] struct {
	at   map[uint64][]A
	last uint64
}

// At sets an action to run at the tick. A tick that is not in the future is the next one.
func (s *Scheduler[A]) At(tick uint64, a A) {
	if tick <= s.last {
		tick = s.last + 1
	}
	if s.at == nil {
		s.at = make(map[uint64][]A)
	}
	s.at[tick] = append(s.at[tick], a)
}

// Due returns what runs at the tick, in the order it was set. Ticks are asked for in rising order.
func (s *Scheduler[A]) Due(tick uint64) []A {
	s.last = tick
	out := s.at[tick]
	delete(s.at, tick)
	return out
}

// Pending returns how many actions are set for a later tick.
func (s *Scheduler[A]) Pending() int {
	n := 0
	for _, as := range s.at {
		n += len(as)
	}
	return n
}

// Find returns the action that the key names and that waits for the earliest tick.
func (s *Scheduler[A]) Find(key ActionKey) (A, bool) {
	var found A
	var foundAt uint64
	ok := false
	for tick, actions := range s.at {
		for _, a := range actions {
			if key.Names(a) && (!ok || tick < foundAt) {
				found, foundAt, ok = a, tick, true
				break // the first of this tick is the earliest of it
			}
		}
	}
	return found, ok
}

// Cancel takes away the actions that the key names and that wait for a later tick, and returns how
// many.
func (s *Scheduler[A]) Cancel(key ActionKey) int {
	n := 0
	for tick, actions := range s.at {
		kept := actions[:0:0]
		for _, a := range actions {
			if key.Names(a) {
				n++
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			delete(s.at, tick)
		} else if len(kept) != len(actions) {
			s.at[tick] = kept
		}
	}
	return n
}

// FindWaiting returns the action that the key names among what the engine set for the next tick
// and what waits in the scheduler. What was set for the next tick is looked at first.
func FindWaiting[A Action](s *Scheduler[A], next []A, key ActionKey) (A, bool) {
	for _, a := range next {
		if key.Names(a) {
			return a, true
		}
	}
	return s.Find(key)
}

// Cancel says that the actions a key names, and that wait, are no longer to run. What an action
// cancels is what waits in the scheduler, and what the engine set before the cancel in this same
// tick (the first Next of the actions set for the next tick, the first Later of those set for a
// later tick); what is set after the cancel is not touched, so an action can cancel what waits and
// set another in its place.
type Cancel struct {
	Key         ActionKey
	Next, Later int
}

// SettleSelf does, at the end of a tick, what the actions of the tick set for the engine: the
// cancels first, in the order they were made, and then the actions for a later tick go into the
// scheduler. It returns the actions set for the next tick that were not cancelled.
func SettleSelf[A Action](
	s *Scheduler[A], next []A, later []Timed[A], cancels []Cancel,
) []A {
	nextGone, laterGone := make([]bool, len(next)), make([]bool, len(later))
	for _, c := range cancels {
		s.Cancel(c.Key)
		for i := 0; i < min(c.Next, len(next)); i++ {
			nextGone[i] = nextGone[i] || c.Key.Names(next[i])
		}
		for i := 0; i < min(c.Later, len(later)); i++ {
			laterGone[i] = laterGone[i] || c.Key.Names(later[i].Action)
		}
	}
	for i, t := range later {
		if !laterGone[i] {
			s.At(t.At, t.Action)
		}
	}
	if len(cancels) == 0 {
		return next
	}
	var kept []A
	for i, a := range next {
		if !nextGone[i] {
			kept = append(kept, a)
		}
	}
	return kept
}
