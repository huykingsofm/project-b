package action

// Scheduler keeps what an engine runs at a later tick: the actions that were set for one tick, and
// its crons. The tick count it knows is the last one it gave out, so an action set for a tick that
// has passed runs at the next one.
type Scheduler[A any] struct {
	at    map[uint64][]A
	crons []Cron[A]
	last  uint64
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

// AddCron makes the actions run every every ticks, from tick every on.
func (s *Scheduler[A]) AddCron(every uint64, actions ...A) {
	s.crons = append(s.crons, Cron[A]{Every: every, Actions: actions})
}

// Due returns what runs at the tick: first what was set for it, in the order it was set, then the
// actions of the crons that are due, in the order the crons were added. Ticks are asked for in
// rising order.
func (s *Scheduler[A]) Due(tick uint64) []A {
	s.last = tick
	out := s.at[tick]
	delete(s.at, tick)
	for _, c := range s.crons {
		if c.IsDue(tick) {
			out = append(out, c.Actions...)
		}
	}
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
