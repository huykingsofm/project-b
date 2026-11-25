package configengine

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// UpdateConfigAction sets the variables of Set and puts the variables of Unset back to their
// default (see State.ApplyConfigChange). An action does not fail: a change that the configuration
// refuses is logged and changes nothing.
type UpdateConfigAction struct {
	Set   []Override
	Unset []string
}

func (UpdateConfigAction) ID() sim.ActionID { return UpdateConfigActionID }
func (UpdateConfigAction) Priority() int16  { return 0 }
func (UpdateConfigAction) Actor() any       { return nil }

// Clone returns a copy that shares neither list with the action.
func (a UpdateConfigAction) Clone() sim.Action[*State] {
	return UpdateConfigAction{Set: slices.Clone(a.Set), Unset: slices.Clone(a.Unset)}
}

// Apply changes the configuration.
func (a UpdateConfigAction) Apply(
	_ *sim.Context, s *State, _ *sim.Registry[*State], _ *sim.Plan[*State],
) {
	if err := s.ApplyConfigChange(a.Set, a.Unset); err != nil {
		logging.Warn("the configuration engine refused a change", "err", err)
	}
}

// Encode writes the fields of the action.
func (a UpdateConfigAction) Encode(w *binary.Writer) { encodeChange(w, a.Set, a.Unset) }

func decodeUpdateConfigAction(r *binary.Reader, _ codec.Version) (UpdateConfigAction, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigAction{Set: set, Unset: unset}, err
}

// encodeChange writes what an action and a command of the configuration both hold.
func encodeChange(w *binary.Writer, set []Override, unset []string) {
	w.Uvarint(uint64(len(set)))
	for _, o := range set {
		w.String(o.Name)
		w.String(o.Text)
	}
	w.Uvarint(uint64(len(unset)))
	for _, name := range unset {
		w.String(name)
	}
}

// decodeChange reads what encodeChange wrote.
func decodeChange(r *binary.Reader) ([]Override, []string, error) {
	var set []Override
	var unset []string
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if n > uint64(r.Left()) { // an override takes at least two bytes
		return nil, nil, fmt.Errorf("%w: %d overrides", binary.ErrShort, n)
	}
	for range n {
		set = append(set, Override{Name: r.String(), Text: r.String()})
	}
	m := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if m > uint64(r.Left()) { // a name takes at least one byte
		return nil, nil, fmt.Errorf("%w: %d names", binary.ErrShort, m)
	}
	for range m {
		unset = append(unset, r.String())
	}
	return set, unset, r.Err()
}
