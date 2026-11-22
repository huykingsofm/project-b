package engine

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
)

const poolTestID action.ID = 60001

type poolTestAction struct{}

func (*poolTestAction) ID() action.ID   { return poolTestID }
func (*poolTestAction) Priority() int16 { return 0 }

func TestAPlanFromThePoolIsEmptyAndReadsTheConfigurationItIsGiven(t *testing.T) {
	var pool PlanPool[*poolTestAction]
	p := pool.Get(config.Defaults())
	p.Append(&poolTestAction{})
	p.Next()
	p.Append(&poolTestAction{})
	pool.Put(p)

	cfg, _, err := config.Build(1, map[string]string{"action.max_depth": "1"})
	if err != nil {
		t.Fatal(err)
	}
	q := pool.Get(cfg)
	if q.Len() != 0 || len(q.Cancelled()) != 0 || len(q.TooDeep()) != 0 {
		t.Fatalf("a plan from the pool holds %d entries, %d cancelled, %d cut", q.Len(), len(q.Cancelled()), len(q.TooDeep()))
	}
	if _, ok := q.Next(); ok {
		t.Fatal("a plan from the pool had something to run")
	}
	// the configuration is the one given: with a maximum depth of 1 the command and its child run, and
	// the grandchild is cut.
	q.Append(&poolTestAction{})
	ran := 0
	for {
		if _, ok := q.Next(); !ok {
			break
		}
		ran++
		q.Append(&poolTestAction{}) // every action that runs adds one
	}
	if ran != 2 || len(q.TooDeep()) != 1 {
		t.Fatalf("ran %d, cut %d; want 2 and 1 with a maximum depth of 1", ran, len(q.TooDeep()))
	}
}

func TestAPlanThatWasPutBackHoldsNoAction(t *testing.T) {
	var pool PlanPool[*poolTestAction]
	p := pool.Get(config.Defaults())
	p.Append(&poolTestAction{})
	p.Append(&poolTestAction{})
	pool.Put(p)
	if p.Len() != 0 || len(p.Find(action.PlanKey{ID: poolTestID})) != 0 {
		t.Fatal("a plan that was put back still holds an action")
	}
}
