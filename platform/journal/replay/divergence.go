package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
)

// divergences collects the first difference of each engine.
type divergences struct {
	order []sim.StateID
	first map[sim.StateID]string
	// name says what an engine is called in a difference: its type and its ID. Without it, its ID.
	name func(sim.StateID) string
}

func (d *divergences) add(engine sim.StateID, format string, args ...any) {
	if d.first == nil {
		d.first = map[sim.StateID]string{}
	}
	if _, has := d.first[engine]; has {
		return
	}
	who := fmt.Sprint(engine)
	if d.name != nil {
		who = d.name(engine)
	}
	d.first[engine] = who + ": " + fmt.Sprintf(format, args...)
	d.order = append(d.order, engine)
}

func (d *divergences) list() []string {
	var out []string
	for _, e := range d.order {
		out = append(out, d.first[e])
	}
	return out
}
