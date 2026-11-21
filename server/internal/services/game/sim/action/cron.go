package action

// Cron is a set of actions that are added to the plan every Every ticks.
type Cron[A any] struct {
	Every   uint64
	Actions []A
}

// IsDue reports whether the cron runs at the tick.
func (c Cron[A]) IsDue(tick uint64) bool { return c.Every > 0 && tick%c.Every == 0 }
