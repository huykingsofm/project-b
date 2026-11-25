package configengine

import (
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// UpdateConfigCommand is what a config service sends the configuration engine when the
// configuration changed: the variables that now have another value, with the text of it, and the
// variables that went back to their default.
type UpdateConfigCommand struct {
	Set   []Override
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
	wanted, _, err := config.Build(0, document)
	if err != nil {
		return UpdateConfigCommand{}, false, err
	}
	var c UpdateConfigCommand
	for _, name := range wanted.Diff(current) {
		if text, ok := document[name]; ok {
			c.Set = append(c.Set, Override{Name: name, Text: text})
		} else {
			c.Unset = append(c.Unset, name)
		}
	}
	return c, len(c.Set)+len(c.Unset) > 0, nil
}

func (UpdateConfigCommand) ID() sim.CommandID { return UpdateConfigCommandID }

// Encode writes the fields of the command.
func (c UpdateConfigCommand) Encode(w *binary.Writer) error {
	encodeChange(w, c.Set, c.Unset)
	return nil
}

// Translate makes the action that does what the command says.
func (c UpdateConfigCommand) Translate() []sim.Action[*State] {
	return []sim.Action[*State]{UpdateConfigAction{Set: c.Set, Unset: c.Unset}}
}

// Clone returns a copy that shares neither list with the command.
func (c UpdateConfigCommand) Clone() sim.Command[*State] {
	return UpdateConfigCommand{Set: slices.Clone(c.Set), Unset: slices.Clone(c.Unset)}
}

func decodeUpdateConfigCommand(r *binary.Reader, _ codec.Version) (UpdateConfigCommand, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigCommand{Set: set, Unset: unset}, err
}
