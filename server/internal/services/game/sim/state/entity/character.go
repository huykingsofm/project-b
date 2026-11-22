package entity

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterEntity is the body of a character in a zonelayer. Which character it is the body of is
// in the state of the zonelayer, not here.
type CharacterEntity struct {
	EntityID state.EntityID
	Pos      state.Vec
	Rad      int32
	Hp       int32
	MaxHp    int32
}

func (e CharacterEntity) ID() state.EntityID  { return e.EntityID }
func (e CharacterEntity) KindID() KindID      { return CharacterKindID }
func (e CharacterEntity) Position() state.Vec { return e.Pos }
func (e CharacterEntity) Radius() int32       { return e.Rad }
func (e CharacterEntity) BoundingBox() state.Rect {
	return boundingBoxAround(e.Pos, e.Rad)
}
func (e CharacterEntity) Health() int32    { return e.Hp }
func (e CharacterEntity) MaxHealth() int32 { return e.MaxHp }

func (e CharacterEntity) OnDamage(amount int32) DamageableEntity {
	e.Hp = max(e.Hp-amount, 0)
	return e
}

func (e CharacterEntity) OnHeal(amount int32) DamageableEntity {
	e.Hp = min(e.Hp+amount, e.MaxHp)
	return e
}

func (e CharacterEntity) Hash(h *state.Hasher) {
	hashCommon(h, e)
	h.U64(uint64(uint32(e.Hp)))
	h.U64(uint64(uint32(e.MaxHp)))
}

func (e CharacterEntity) MovedTo(pos state.Vec) Entity {
	e.Pos = pos
	return e
}
