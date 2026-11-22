package sim

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func mustCodecs(t testing.TB) *Codecs {
	t.Helper()
	c, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAnActionComesBackAsItWasWritten(t *testing.T) {
	c := mustCodecs(t)
	for _, a := range []BaseAction{&addAction{N: -5}, &anonymousAction{name: "x", priority: -9}} {
		var w binarycodec.Writer
		if err := EncodeAction(&w, a); err != nil {
			t.Fatalf("%T: %v", a, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := c.DecodeAction(r, 1)
		if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, a) {
			t.Fatalf("%T came back as %+v, error %v, %d bytes left", a, got, err, r.Left())
		}
	}
}

func TestACodecRefusesANumberOrATypeThatTwoKindsShare(t *testing.T) {
	c := mustCodecs(t)
	if err := c.RegisterAction[*toyState](&addAction{}, decodeAddAction); err == nil {
		t.Fatal("the same action was registered twice")
	}
	if err := c.RegisterState[*toyState](&toyState{}, decodeToyState); err == nil {
		t.Fatal("the same state was registered twice")
	}
	if err := c.RegisterOnFail(testDrop{}, decodeTestDrop); err == nil {
		t.Fatal("the same OnFail was registered twice")
	}
	// another Go type that says it has a number that is taken
	taken := &otherAction{}
	if err := c.RegisterAction[*otherState](taken, decodeOtherAction); err == nil {
		t.Fatal("an action was registered under a number that was taken")
	}
}

func toyMessages() []Message {
	add := func(n int64) Action[*toyState] { return &addAction{N: n} }
	undo := NewActionMessage[*toyState](5, testDrop{}, add(1))
	return []Message{
		NewActionMessage[*toyState](5, testDrop{}, add(1), add(2)).
			Stamp(Header{From: 2, Seq: 77}).Again(),
		NewActionMessage[*toyState](5, testDrop{}),
		NewActionMessage[*toyState](5, testCompensate{With: undo}, add(3)),
		NewActionMessage[*toyState](
			5, testCompensate{With: NewActionMessage[*toyState](1, testCompensate{With: undo})},
		),
		NewActionMessage[*otherState](9, testDrop{}, &otherAction{}),
	}
}

func TestAMessageComesBackWholeWithItsOnFailAndTheMessageInsideIt(t *testing.T) {
	c := mustCodecs(t)
	for i, m := range toyMessages() {
		var w binarycodec.Writer
		if err := EncodeMessage(&w, m); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := c.DecodeMessage(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("message %d: error %v, %d bytes left", i, err, r.Left())
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("message %d came back as %+v, want %+v", i, got, m)
		}
	}
}

func TestBytesThatAreCutOffOrWrongGiveAnErrorForAMessageAndNeverAPanic(t *testing.T) {
	c := mustCodecs(t)
	for i, m := range toyMessages() {
		var w binarycodec.Writer
		if err := EncodeMessage(&w, m); err != nil {
			t.Fatal(err)
		}
		for n := range w.B { // every proper prefix
			if _, err := c.DecodeMessage(binarycodec.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("message %d: %d of %d bytes were read", i, n, len(w.B))
			}
		}
		for at := range w.B { // every byte made something else
			bad := append([]byte(nil), w.B...)
			bad[at] ^= 0xff
			c.DecodeMessage(binarycodec.NewReader(bad), 1) // an error or a message, no panic
		}
	}
}

func TestAMessageWithAnActionOfAnotherStateTypeOrTooDeepIsRefused(t *testing.T) {
	c := mustCodecs(t)
	var w binarycodec.Writer
	w.Uvarint(uint64(toyType)) // the message says it is for a toy state
	w.Uvarint(1)               // to
	w.Uvarint(0)
	w.Uvarint(0) // the header: the sender and the number
	w.Uvarint(0) // attempts
	w.Uvarint(uint64(testDropID))
	w.Uvarint(1)
	if err := EncodeAction(&w, &otherAction{}); err != nil { // an action of the other type
		t.Fatal(err)
	}
	_, err := c.DecodeMessage(binarycodec.NewReader(w.B), 1)
	if !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("an action of another type of state: %v, want ErrUnknownCode", err)
	}

	nested := func(n int) Message {
		m := NewActionMessage[*toyState](1, testDrop{})
		for range n {
			m = NewActionMessage[*toyState](1, testCompensate{With: m})
		}
		return m
	}
	// 8 messages inside one another is the most that can be read back, and it is written
	var fits binarycodec.Writer
	if err := EncodeMessage(&fits, nested(8)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DecodeMessage(binarycodec.NewReader(fits.B), 1); err != nil {
		t.Fatalf("messages nested 8 deep: %v", err)
	}
	// 9 is refused when it is written, and nothing of it is left: what is written is readable
	var d binarycodec.Writer
	err = EncodeMessage(&d, nested(9))
	if !errors.Is(err, binarycodec.ErrBadValue) || len(d.B) != 0 {
		t.Fatalf("messages nested 9 deep were written: %v, %d bytes", err, len(d.B))
	}
	// and bytes of 9 are refused when they are read (Encode of the message does not look at depth)
	var raw binarycodec.Writer
	if err := nested(9).Encode(&raw); err != nil {
		t.Fatal(err)
	}
	_, err = c.DecodeMessage(binarycodec.NewReader(raw.B), 1)
	if !errors.Is(err, binarycodec.ErrBadValue) {
		t.Fatalf("bytes of messages nested 9 deep: %v, want ErrBadValue", err)
	}
}

// noEncodeAction is an action that cannot be written.
type noEncodeAction struct{}

func (noEncodeAction) ID() ActionID    { return addID }
func (noEncodeAction) Priority() int16 { return 0 }
func (noEncodeAction) Apply(*Context, *toyState, *Registry[*toyState], *Plan[*toyState]) {
}

func TestAnActionWithNoEncodeAndAMessageThatHoldsOneAreNotWrittenAtAll(t *testing.T) {
	var w binarycodec.Writer
	err := EncodeAction(&w, noEncodeAction{})
	if !errors.Is(err, ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("an action with no Encode: %v, %d bytes written", err, len(w.B))
	}
	m := NewActionMessage[*toyState](1, testDrop{}, &addAction{}, noEncodeAction{})
	if err := EncodeMessage(&w, m); !errors.Is(err, ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a message with such an action: %v, %d bytes written", err, len(w.B))
	}
	if err := EncodeMessage(&w, NewActionMessage[*toyState](1, nil)); !errors.Is(err, ErrNoCodec) {
		t.Fatalf("a message with no OnFail: %v, want ErrNoCodec", err)
	}
}

func TestACommandAndAStateComeBackAsTheyWereWritten(t *testing.T) {
	c := mustCodecs(t)
	var w binarycodec.Writer
	if err := EncodeCommand(&w, addCommand{N: 12}); err != nil {
		t.Fatal(err)
	}
	got, err := c.DecodeCommand(binarycodec.NewReader(w.B), 1)
	if err != nil || got != (addCommand{N: 12}) {
		t.Fatalf("the command came back as %+v, error %v", got, err)
	}
	if _, ok := got.(Command[*toyState]); !ok {
		t.Fatal("the command is not a command of the toy state")
	}
	if _, ok := got.(Command[*otherState]); ok {
		t.Fatal("the command is a command of another state")
	}

	w = binarycodec.Writer{}
	st := &toyState{id: 4, tick: 9, Total: -3}
	if err := EncodeState(&w, st); err != nil {
		t.Fatal(err)
	}
	read, err := c.DecodeState(binarycodec.NewReader(w.B), 1)
	if err != nil || !reflect.DeepEqual(read, st) {
		t.Fatalf("the state came back as %+v, error %v", read, err)
	}
	_, err = c.DecodeState(binarycodec.NewReader([]byte{99}), 1)
	if !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("a state of a type that is not registered: %v", err)
	}
	_, err = c.DecodeCommand(binarycodec.NewReader([]byte{99}), 1)
	if !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("a command that is not registered: %v", err)
	}
}

type fixedReader map[StateID]State

func (f fixedReader) Read(id StateID) (State, error) {
	if s, ok := f[id]; ok {
		return s, nil
	}
	return nil, ErrStateNotFound
}

func TestARegistryReadsAStateByItsIDAndTellsNotFoundFromWrongType(t *testing.T) {
	r := &Registry[*toyState]{Stable: fixedReader{1: &toyState{id: 1}, 2: &otherState{}}}
	if got, err := r.Get[*toyState](1); err != nil || got.StateID() != 1 {
		t.Fatalf("a state that is there: %v, %v", got, err)
	}
	if _, err := r.Get[*toyState](3); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("a state that is not there: %v", err)
	}
	if _, err := r.Get[*toyState](2); !errors.Is(err, ErrStateWrongType) {
		t.Fatalf("a state of another type: %v", err)
	}
	// an interface of the read methods is asked for as well as the type itself
	if _, err := r.Get[interface{ Hash() uint64 }](2); err != nil {
		t.Fatalf("an interface that the state has: %v", err)
	}
}

func TestSendPutsAMessageForTheStateInTheOutboxInOrder(t *testing.T) {
	var r Registry[*toyState]
	r.Send[*otherState](9, testDrop{}, &otherAction{})
	r.Send[*toyState](5, testDrop{}, &addAction{N: 1})
	sent := r.Take()
	if len(sent) != 2 || sent[0].Dest() != 9 || sent[1].Dest() != 5 || r.Take() != nil {
		t.Fatalf("the outbox held %+v", sent)
	}
}
