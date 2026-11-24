package engine

import (
	"testing"
)

func TestAPlanFromThePoolIsEmpty(t *testing.T) {
	var pool PlanPool[*toyState]
	p := pool.Get()
	p.Append(toyAction{})
	p.Next(4)
	p.Append(toyAction{})
	pool.Put(p)

	q := pool.Get()
	if q.Len() != 0 || len(q.Cancelled()) != 0 {
		t.Fatalf("a plan from the pool holds %d entries, %d cancelled", q.Len(), len(q.Cancelled()))
	}
	if _, ok := q.Next(1); ok {
		t.Fatal("a plan from the pool had something to run")
	}
	// with a maximum depth of 1 the command and its child run,
	// and the grandchild is cut.
	q.Append(toyAction{})
	ran := 0
	for {
		if _, ok := q.Next(1); !ok {
			break
		}
		ran++
		q.Append(toyAction{}) // every action that runs adds one
	}
	if cut := q.Len() - ran; ran != 2 || cut != 1 {
		t.Fatalf("ran %d, cut %d; want 2 and 1 with a maximum depth of 1", ran, cut)
	}
}

func TestAPlanThatWasPutBackHoldsNoAction(t *testing.T) {
	var pool PlanPool[*toyState]
	p := pool.Get()
	p.Append(toyAction{})
	p.Append(toyAction{})
	pool.Put(p)
	found := 0
	for range p.FindByID[toyAction](toyActionID) {
		found++
	}
	if p.Len() != 0 || found != 0 {
		t.Fatal("a plan that was put back still holds an action")
	}
}
