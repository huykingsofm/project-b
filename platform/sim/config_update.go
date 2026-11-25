package sim

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// UpdateConfigCommand is what a config service sends the configuration engine when the
// configuration changed: the variables that now have another value, with the text of it, and the
// variables that went back to their default.
type UpdateConfigCommand struct {
	Set   []ConfigOverride
	Unset []string
}

// NewUpdateConfigCommand makes the command that takes the configuration current has to the one the
// document says: the text of the JSON value of each variable that it sets, by name; the others are
// at their default. It says false, with no command, when no variable would change. It returns an
// error, with no command, when the document is not a configuration: the names that no variable has
// are left out, as config.Build does.
func NewUpdateConfigCommand(
	current config.ReadOnlySnapshot, document map[string]string,
) (UpdateConfigCommand, bool, error) {
	wanted, err := config.Build(document)
	if err != nil {
		return UpdateConfigCommand{}, false, err
	}
	var c UpdateConfigCommand
	for _, name := range wanted.Diff(current) {
		if text, ok := document[name]; ok {
			c.Set = append(c.Set, ConfigOverride{Name: name, Text: text})
		} else {
			c.Unset = append(c.Unset, name)
		}
	}
	return c, len(c.Set)+len(c.Unset) > 0, nil
}

func (UpdateConfigCommand) ID() CommandID { return UpdateConfigCommandID }

// Encode writes the fields of the command.
func (c UpdateConfigCommand) Encode(w *binary.Writer) error {
	encodeChange(w, c.Set, c.Unset)
	return nil
}

// Translate makes the action that does what the command says.
func (c UpdateConfigCommand) Translate() []Action[*ConfigState] {
	return []Action[*ConfigState]{UpdateConfigAction{Set: c.Set, Unset: c.Unset}}
}

// Clone returns a copy that shares neither list with the command.
func (c UpdateConfigCommand) Clone() Command[*ConfigState] {
	return UpdateConfigCommand{Set: slices.Clone(c.Set), Unset: slices.Clone(c.Unset)}
}

func decodeUpdateConfigCommand(r *binary.Reader, _ codec.Version) (UpdateConfigCommand, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigCommand{Set: set, Unset: unset}, err
}

// UpdateConfigAction sets the variables of Set and puts the variables of Unset back to their
// default (see ConfigState.ApplyConfigChange). An action does not fail: a change that the
// configuration refuses is logged and changes nothing.
type UpdateConfigAction struct {
	Set   []ConfigOverride
	Unset []string
}

func (UpdateConfigAction) ID() ActionID    { return UpdateConfigActionID }
func (UpdateConfigAction) Priority() int16 { return 0 }
func (UpdateConfigAction) Actor() any      { return nil }

// Clone returns a copy that shares neither list with the action.
func (a UpdateConfigAction) Clone() Action[*ConfigState] {
	return UpdateConfigAction{Set: slices.Clone(a.Set), Unset: slices.Clone(a.Unset)}
}

// Apply changes the configuration.
func (a UpdateConfigAction) Apply(
	_ *Context, s *ConfigState, _ *Registry[*ConfigState], _ *Plan[*ConfigState],
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
func encodeChange(w *binary.Writer, set []ConfigOverride, unset []string) {
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
func decodeChange(r *binary.Reader) ([]ConfigOverride, []string, error) {
	var set []ConfigOverride
	var unset []string
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if n > uint64(r.Left()) { // an override takes at least two bytes
		return nil, nil, fmt.Errorf("%w: %d overrides", binary.ErrShort, n)
	}
	for range n {
		set = append(set, ConfigOverride{Name: r.String(), Text: r.String()})
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
