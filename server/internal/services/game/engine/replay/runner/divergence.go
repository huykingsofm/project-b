package runner

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// divergences collects the first difference of each engine.
type divergences struct {
	order []message.Address
	first map[message.Address]string
}

func (d *divergences) add(engine message.Address, format string, args ...any) {
	if d.first == nil {
		d.first = map[message.Address]string{}
	}
	if _, has := d.first[engine]; has {
		return
	}
	d.first[engine] = fmt.Sprintf("%v: ", engine) + fmt.Sprintf(format, args...)
	d.order = append(d.order, engine)
}

func (d *divergences) list() []string {
	var out []string
	for _, e := range d.order {
		out = append(out, d.first[e])
	}
	return out
}
