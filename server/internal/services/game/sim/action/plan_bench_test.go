package action

import (
	"fmt"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// planActions is the actions of a tick as an engine sees them: moves and attacks of characters
// (they have an author), then the damage that the attacks make and the regens.
func planActions(n int) []ZoneLayerAction {
	out := make([]ZoneLayerAction, 0, n)
	for i := range n {
		id := state.CharacterID(1000 + i%200)
		switch i % 5 {
		case 0, 1:
			out = append(out, &ZoneLayerCharacterMoveAction{CharacterID: id})
		case 2:
			out = append(out, &ZoneLayerCharacterNormalAttackTriggerAction{CharacterID: id})
		case 3:
			out = append(out, &ZoneLayerCharacterDamageAction{Amount: 1})
		default:
			out = append(out, &ZoneLayerCharacterRegenAction{})
		}
	}
	return out
}

func filledPlan(n int) *Plan[ZoneLayerAction] {
	p := NewPlan[ZoneLayerAction]()
	for _, a := range planActions(n) {
		p.Append(a)
	}
	return p
}

var planSizes = []int{20, 100, 1000}

// BenchmarkPlanAppend fills a plan that is reused, as an engine does every tick.
func BenchmarkPlanAppend(b *testing.B) {
	cfg := config.Defaults()
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			actions := planActions(n)
			p := NewPlan[ZoneLayerAction]()
			b.ReportAllocs()
			for b.Loop() {
				p.Reset(cfg)
				for _, a := range actions {
					p.Append(a)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/action")
		})
	}
}

// BenchmarkPlanFind looks the damage up by its ID with Find, which makes a slice.
func BenchmarkPlanFind(b *testing.B) {
	for _, n := range planSizes {
		b.Run(fmt.Sprintf("actions=%d", n), func(b *testing.B) {
			p := filledPlan(n)
			b.ReportAllocs()
			for b.Loop() {
				if len(p.Find(ActionKey{ID: ZoneLayerCharacterDamageActionID})) == 0 {
					b.Fatal("no damage found")
				}
			}
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
				for _, d := range p.FindByID[*ZoneLayerCharacterDamageAction](
					ZoneLayerCharacterDamageActionID,
				) {
					amount += d.Amount
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
				for range p.FindByID[*ZoneLayerCharacterDamageAction](
					ZoneLayerCharacterDamageActionID,
				) {
					if outer++; outer > 3 {
						break
					}
					for range p.FindByID[*ZoneLayerCharacterRegenAction](
						ZoneLayerCharacterRegenActionID,
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
