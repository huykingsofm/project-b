package zonelayer

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// CharacterEntity is the body of a character in a zonelayer. Which character it is the body of is
// in the state of the zonelayer, not here.
type CharacterEntity struct {
	entityID state.EntityID
	pos      state.Vec
	rad      int32
	hp       int32
	maxHp    int32
}

func (e CharacterEntity) ID() state.EntityID  { return e.entityID }
func (e CharacterEntity) KindID() KindID      { return CharacterKindID }
func (e CharacterEntity) Position() state.Vec { return e.pos }
func (e CharacterEntity) Radius() int32       { return e.rad }
func (e CharacterEntity) BoundingBox() state.Rect {
	return boundingBoxAround(e.pos, e.rad)
}
func (e CharacterEntity) Health() int32    { return e.hp }
func (e CharacterEntity) MaxHealth() int32 { return e.maxHp }

func (e CharacterEntity) OnDamage(amount int32) DamageableEntity {
	e.hp = max(e.hp-amount, 0)
	return e
}

func (e CharacterEntity) OnHeal(amount int32) DamageableEntity {
	e.hp = min(e.hp+amount, e.maxHp)
	return e
}

func (e CharacterEntity) Hash(h *sim.Hasher) {
	hashCommon(h, e)
	h.U64(uint64(uint32(e.hp)))
	h.U64(uint64(uint32(e.maxHp)))
}

func (e CharacterEntity) MovedTo(pos state.Vec) Entity {
	e.pos = pos
	return e
}

func (e CharacterEntity) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(e.entityID))
	e.pos.Encode(w)
	w.Varint(int64(e.rad))
	w.Varint(int64(e.hp))
	w.Varint(int64(e.maxHp))
}

func decodeCharacterEntity(r *binarycodec.Reader, version binarycodec.Version) (Entity, error) {
	e := CharacterEntity{
		entityID: state.EntityID(r.Uvarint()),
		pos:      state.DecodeVec(r, version),
		rad:      int32(r.Varint()),
		hp:       int32(r.Varint()),
		maxHp:    int32(r.Varint()),
	}
	return e, r.Err()
}
