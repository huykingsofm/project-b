package sim_test

import (
	"fmt"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

// The kinds of action of the plan of a tick: a move and an attack of a character (they have an
// author), the damage that the attack makes, and a regen.
const (
	benchMoveID   sim.ActionID = 60010
	benchAttackID sim.ActionID = 60011
	benchDamageID sim.ActionID = 60012
	benchRegenID  sim.ActionID = 60013
)

type benchAction struct {
	anonymousAction
	id     sim.ActionID
	author int
	amount int32
}

func (a *benchAction) ID() sim.ActionID { return a.id }

type authoredBenchAction struct{ benchAction }

func (a *authoredBenchAction) Actor() any { return a.author }

// planActions is the actions of a tick as an engine sees them: moves and attacks of characters
// (they have an author), then the damage that the attacks make and the regens.
func planActions(n int) []sim.Action[*toyState] {
	out := make([]sim.Action[*toyState], 0, n)
	for i := range n {
		author := 1000 + i%200
		switch i % 5 {
		case 0, 1:
			out = append(out, &authoredBenchAction{benchAction{id: benchMoveID, author: author}})
		case 2:
			out = append(out, &authoredBenchAction{benchAction{id: benchAttackID, author: author}})
		case 3:
			out = append(out, &benchAction{id: benchDamageID, amount: 1})
		default:
			out = append(out, &benchAction{id: benchRegenID})
		}
	}
	return out
}

func filledPlan(n int) *sim.Plan[*toyState] {
	p := sim.NewPlan[*toyState]()
	for _, a := range planActions(n) {
		p.Append(a)
	}
	return p
}

var planSizes = []int{20, 100, 1000}

// BenchmarkPlanAppend fills a plan that is reused, as an engine does every tick.
func BenchmarkPlanAppend(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			actions := planActions(n)
			p := sim.NewPlan[*toyState]()
			b.ReportAllocs()
			for b.Loop() {
				p.Reset()
				for _, a := range actions {
					p.Append(a)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/action")
		})
	}
}

// BenchmarkPlanAppendAndRun fills a plan with Append and takes every entry out: what an engine does
// with the actions it starts a tick with, one by one.
func BenchmarkPlanAppendAndRun(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			actions := planActions(n)
			p := sim.NewPlan[*toyState]()
			b.ReportAllocs()
			for b.Loop() {
				p.Reset()
				for _, a := range actions {
					p.Append(a)
				}
				for {
					if _, ok := p.Next(4); !ok {
						break
					}
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/action")
		})
	}
}

// BenchmarkPlanBuildAndRun does the same with Build, which orders the actions once.
func BenchmarkPlanBuildAndRun(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			actions := planActions(n)
			p := sim.NewPlan[*toyState]()
			p.Reset()
			b.ReportAllocs()
			for b.Loop() {
				p.Build(actions)
				for {
					if _, ok := p.Next(4); !ok {
						break
					}
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/action")
		})
	}
}

// BenchmarkPlanFindByID walks all the damage with FindByID: what a shield does for each shield
// when no damage is for its character.
func BenchmarkPlanFindByID(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			p := filledPlan(n)
			b.ReportAllocs()
			for b.Loop() {
				amount := int32(0)
				for _, d := range p.FindByID[*benchAction](benchDamageID) {
					amount += d.amount
				}
				if amount == 0 {
					b.Fatal("no damage found")
				}
			}
		})
	}
}

// BenchmarkPlanFindByIDNested walks the damage and, for each of the first 3, the regens inside it,
// as an effect that reacts to an action that reacts to another does.
func BenchmarkPlanFindByIDNested(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			p := filledPlan(n)
			b.ReportAllocs()
			for b.Loop() {
				found, outer := 0, 0
				for range p.FindByID[*benchAction](
					benchDamageID,
				) {
					if outer++; outer > 3 {
						break
					}
					for range p.FindByID[*benchAction](
						benchRegenID,
					) {
						found++
					}
				}
				if found == 0 {
					b.Fatal("nothing found")
				}
			}
		})
	}
}
