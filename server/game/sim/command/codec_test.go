package command

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// aCharacter is a character of full health, as NewCharacter makes it.
func aCharacter(id state.CharacterID, name string) zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(id, name, 100, 250, 256)
	if err != nil {
		panic(err)
	}
	return c
}

// characterWithCooldowns is a character that has been playing: it has cooldowns, in an order that
// is not the order of the numbers of their keys.
func characterWithCooldowns() zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(7, "Binh Nguyên", 100, 250, 256)
	if err != nil {
		panic(err)
	}
	c.SetHealth(80)
	c.SetCooldown(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 1_000_006)
	c.SetCooldown(action.ZoneLayerCharacterMoveActionCooldownKey, 52_004)
	return c
}

// examples has a command of every kind.
var examples = []sim.BaseCommand{
	WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"},
	ZoneCharacterEnterCommand{Character: characterWithCooldowns()},
	ZoneCharacterEnterCommand{
		Character: aCharacter(1, "An"),
	},
	ZoneChatSendCommand{FromID: 1, Text: "hello zone"},
	ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 5},
	ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
	ZoneLayerChatSendCommand{FromID: 1, Text: "xin chào"},
	ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 9},
	ZoneLayerCharacterItemPickupCommand{CharacterID: 2, ItemEntityID: 1 << 40},
	ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100},
}

func codecs(t testing.TB) *sim.Codecs {
	t.Helper()
	c := sim.NewCodecs()
	if err := Register(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryKindOfCommandComesBackAsItWasWritten(t *testing.T) {
	codecs := codecs(t)
	for _, c := range examples {
		var w binarycodec.Writer
		if err := sim.EncodeCommand(&w, c); err != nil {
			t.Fatalf("%T: %v", c, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := codecs.DecodeCommand(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("%T: error %v, %d bytes left", c, err, r.Left())
		}
		if !reflect.DeepEqual(got, c) {
			t.Fatalf("%T: got %+v, want %+v", c, got, c)
		}
	}
}

func TestTheCooldownsOfACharacterKeepTheirOrder(t *testing.T) {
	codecs := codecs(t)
	var w binarycodec.Writer
	if err := sim.EncodeCommand(
		&w, ZoneCharacterEnterCommand{Character: characterWithCooldowns()},
	); err != nil {
		t.Fatal(err)
	}
	got, _ := codecs.DecodeCommand(binarycodec.NewReader(w.B), 1)
	ch := got.(ZoneCharacterEnterCommand).Character
	keys := ch.CooldownKeys()
	if len(keys) != 2 ||
		keys[0] != any(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey) {
		t.Fatalf("keys %v, want the attack key first as it was set", keys)
	}
}

func TestBytesThatAreCutOffAreAnErrorAndNeverAPanic(t *testing.T) {
	codecs := codecs(t)
	for _, c := range examples {
		var w binarycodec.Writer
		if err := sim.EncodeCommand(&w, c); err != nil {
			t.Fatal(err)
		}
		for cut := 0; cut < len(w.B); cut++ {
			if _, err := codecs.DecodeCommand(binarycodec.NewReader(w.B[:cut]), 1); err == nil {
				t.Fatalf("%T cut at %d of %d bytes was read", c, cut, len(w.B))
			}
		}
	}
}

// strangerCommand has an ID that is not in the register, so it has no decoder and cannot be read
// back.
type strangerCommand struct{}

func (strangerCommand) ID() sim.CommandID                { return 60000 }
func (strangerCommand) Encode(*binarycodec.Writer) error { return nil }

// failingCommand is in the register but cannot be written.
type failingCommand struct{}

func (failingCommand) ID() sim.CommandID { return ZoneChatSendCommandID }
func (failingCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(1) // some bytes before it fails
	return errors.New("cannot write")
}

func TestACommandThatCannotBeWrittenIsRefusedAndWritesNothingAndAnUnknownOneIsRefusedOnRead(
	t *testing.T,
) {
	codecs := codecs(t)
	var w binarycodec.Writer
	w.Uint8(0xee)
	if err := sim.EncodeCommand(&w, failingCommand{}); err == nil {
		t.Fatal("a command whose Encode failed was written")
	}
	if err := sim.EncodeCommand(&w, nil); !errors.Is(err, sim.ErrNoCodec) {
		t.Fatalf("nil: %v", err)
	}
	if len(w.B) != 1 {
		t.Fatalf("%d bytes written by the commands that were refused, want none", len(w.B)-1)
	}
	// a command whose ID is not in the register is written as it says, and refused when it is read
	var stranger binarycodec.Writer
	if err := sim.EncodeCommand(&stranger, strangerCommand{}); err != nil {
		t.Fatal(err)
	}
	_, err := codecs.DecodeCommand(binarycodec.NewReader(stranger.B), 1)
	if !errors.Is(err, sim.ErrUnknownCode) {
		t.Fatalf("a command that has no codec: %v", err)
	}
}

func TestANumberThatIsNotACommandOrTooLargeForACooldownKeyIsAnError(t *testing.T) {
	codecs := codecs(t)
	var w binarycodec.Writer
	w.Uvarint(999)
	_, err := codecs.DecodeCommand(binarycodec.NewReader(w.B), 1)
	if !errors.Is(err, sim.ErrUnknownCode) {
		t.Fatalf("an unknown command: %v", err)
	}
	var w2 binarycodec.Writer
	w2.Uvarint(2) // ZoneCharacterEnterCommand
	w2.Uvarint(1)
	w2.String("x")
	w2.Varint(1)
	w2.Varint(1)
	w2.Varint(1)
	w2.Varint(1)
	w2.Uvarint(1)     // one cooldown
	w2.Uvarint(70000) // a number that is not one of a key: it is more than 16 bits
	w2.Uvarint(5)
	if _, err := codecs.DecodeCommand(binarycodec.NewReader(w2.B), 1); !errors.Is(
		err, zonelayerstate.ErrUnknownCooldownKey,
	) {
		t.Fatalf("a cooldown key that is too large: %v", err)
	}
	var w3 binarycodec.Writer
	w3.Uvarint(2)
	w3.Uvarint(1)
	w3.String("x")
	w3.Varint(1)
	w3.Varint(1)
	w3.Varint(1)
	w3.Varint(1)
	w3.Uvarint(1 << 60) // a count that the bytes cannot hold
	if _, err := codecs.DecodeCommand(binarycodec.NewReader(w3.B), 1); err == nil {
		t.Fatal("a count of cooldowns that the bytes cannot hold was read")
	}
}

// commandTypes returns the names of the types that the files of the package give a Translate
// method: the commands.
func commandTypes(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(
		fset,
		".",
		func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") },
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || !strings.HasPrefix(fd.Name.Name, "Translate") {
					continue
				}
				if id, ok := fd.Recv.List[0].Type.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
			}
		}
	}
	slices.Sort(names)
	return names
}

// registerSize is how many names the constant block of id.go declares.
func registerSize(t *testing.T) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "id.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			for _, spec := range gd.Specs {
				n += len(spec.(*ast.ValueSpec).Names)
			}
		}
	}
	return n
}

// Every command of the package is in examples, so that the tests that go through the examples go
// through all of them.
func TestExamplesHaveACommandOfEveryKindAndTheRegisterAndTheDecodersAgree(t *testing.T) {
	codecs := codecs(t)
	declared := commandTypes(t)
	if len(declared) == 0 {
		t.Fatal("found no commands: the test does not look at the files")
	}
	var listed []string
	seen := map[sim.CommandID]reflect.Type{}
	for _, c := range examples {
		name := reflect.TypeOf(c).Name()
		if !slices.Contains(listed, name) {
			listed = append(listed, name)
		}
		if c.ID() == 0 {
			t.Errorf("%T has the ID 0", c)
		}
		if other, dup := seen[c.ID()]; dup && other != reflect.TypeOf(c) {
			t.Errorf("%v and %T have the ID %d", other, c, c.ID())
		}
		seen[c.ID()] = reflect.TypeOf(c)
		if !codecs.Commands.Has(c.ID()) {
			t.Errorf("%T has the ID %d, which has no decoder", c, c.ID())
		}
	}
	slices.Sort(listed)
	if !slices.Equal(declared, listed) {
		t.Fatalf(
			"the commands declared are\n%v\nand the ones in examples are\n%v", declared, listed,
		)
	}
	if n := registerSize(t); n != len(codecs.Commands.Numbers()) || n != len(declared) {
		t.Fatalf(
			"%d IDs in the register, %d decoders and %d commands: each command has an ID of its "+
				"own, and each ID a decoder",
			n,
			len(codecs.Commands.Numbers()),
			len(declared),
		)
	}
}

func TestTheIDsOfTheCommandsAreFixedForTheFormatOfTheLog(t *testing.T) {
	// A number that changes breaks every log that was written: add commands at the end, never
	// renumber.
	if WorldChatPrivateSendCommandID != 1 || ZoneCharacterEnterCommandID != 2 ||
		ZoneLayerCharacterMoveCommandID != 4 || ZoneLayerCharacterShieldGrantCommandID != 9 {
		t.Fatal("the ID of a command changed")
	}
}

// Commands come from outside, with the goroutine that sends one still holding what it sent, so what
// a command holds is a value or has Clone, and the state that takes it copies it (R29).
func TestEveryCommandHoldsOnlyValuesOrTypesWithClone(t *testing.T) {
	rules := aliasing.Rules{Boundary: true}
	for _, c := range examples {
		if err := aliasing.CheckCloneable(reflect.TypeOf(c), rules); err != nil {
			t.Errorf("%T: %v", c, err)
		}
	}
}

// cloneOf calls the Clone of the type of state that the command is addressed to.
func cloneOf(t *testing.T, c sim.BaseCommand) sim.BaseCommand {
	t.Helper()
	switch c := c.(type) {
	case sim.Command[*zonelayerstate.State]:
		return c.Clone()
	case sim.Command[*zonestate.State]:
		return c.Clone()
	case sim.Command[*worldstate.State]:
		return c.Clone()
	}
	t.Fatalf("%T is a command of no type of state", c)
	return nil
}

func TestTheCloneOfEveryCommandIsTheSameCommandAndSharesNothingWithIt(t *testing.T) {
	for _, c := range examples {
		got := cloneOf(t, c)
		if reflect.TypeOf(got) != reflect.TypeOf(c) {
			t.Errorf("%T cloned into a %T", c, got)
		}
		if !reflect.DeepEqual(got, c) {
			t.Errorf("%T: the clone is %+v, want %+v", c, got, c)
		}
		if err := aliasing.Disjoint(c, got); err != nil {
			t.Errorf("%T shares memory with its clone: %v", c, err)
		}
	}
}

func TestTheCloneOfACommandThatHoldsCooldownsIsNotChangedByItsSender(t *testing.T) {
	sent := ZoneCharacterEnterCommand{Character: characterWithCooldowns()}
	kept := sent.Clone().(ZoneCharacterEnterCommand)
	key := action.ZoneLayerCharacterMoveActionCooldownKey
	sent.Character.SetCooldown(key, 999)
	if until, _ := kept.Character.CooldownUntil(key); until == 999 {
		t.Fatal("the sender changed the clone")
	}
}
