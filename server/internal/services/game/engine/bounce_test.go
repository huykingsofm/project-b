package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

type notice struct {
	actor any
	key   string
	args  []any
}

// bounced runs what a message that came back as Full says to do, on a bouncer that records what it
// is told.
func bounced(
	onFail message.OnFail, attempts int, stopping bool,
) (*bouncer, []notice, message.Message) {
	var told []notice
	b := &bouncer{
		stopping: stopping,
		notice:   func(a any, k string, args []any) { told = append(told, notice{a, k, args}) },
	}
	m := action.MessageToZone(1, onFail, &action.ZoneChatSendAction{FromID: 5, Text: "x"})
	for range attempts {
		m = m.Again()
	}
	b.handle(config.Defaults(), m, message.Full)
	return b, told, m
}

func TestWhatAMessageSaysToDoWhenItIsNotDeliveredIsDoneByItsOwnKind(t *testing.T) {
	max := action.MaxAttempts.Get(config.Defaults())
	other := message.ZoneAddress(2)
	undo := action.MessageToZoneLayer(
		3, onfail.Drop{}, &action.ZoneLayerCharacterRegenAction{},
	)

	t.Run("drop forgets it and counts it", func(t *testing.T) {
		b, _, _ := bounced(onfail.Drop{}, 0, false)
		if b.dropped != 1 || len(b.pending) != 0 {
			t.Fatalf("%d dropped, %d to send", b.dropped, len(b.pending))
		}
	})
	t.Run("retry sends it again, once more tried", func(t *testing.T) {
		b, _, m := bounced(onfail.Retry{}, 0, false)
		if len(b.pending) != 1 || b.pending[0].Attempts() != m.Attempts()+1 ||
			b.pending[0].Dest() != m.Dest() {
			t.Fatalf("to send again: %+v", b.pending)
		}
	})
	t.Run("retry stops at the most a message may be tried", func(t *testing.T) {
		b, _, _ := bounced(onfail.Retry{}, max, false)
		if b.dropped != 1 || len(b.pending) != 0 {
			t.Fatalf("%d dropped, %d to send, at %d attempts", b.dropped, len(b.pending), max)
		}
	})
	t.Run("retry and redirect send nothing while the engine stops", func(t *testing.T) {
		for _, f := range []message.OnFail{onfail.Retry{}, onfail.Redirect{To: other}} {
			b, _, _ := bounced(f, 0, true)
			if b.dropped != 1 || len(b.pending) != 0 {
				t.Fatalf("%T: %d dropped, %d to send", f, b.dropped, len(b.pending))
			}
		}
	})
	t.Run("redirect sends it to the other engine", func(t *testing.T) {
		b, _, m := bounced(onfail.Redirect{To: other}, 0, false)
		if len(b.pending) != 1 || b.pending[0].Dest() != other ||
			b.pending[0].Attempts() != m.Attempts()+1 {
			t.Fatalf("to send: %+v", b.pending)
		}
	})
	t.Run("escalate sends it on, once more tried, up to the most", func(t *testing.T) {
		b, _, m := bounced(onfail.Escalate{To: other}, 0, false)
		if len(b.pending) != 1 || b.pending[0].Attempts() != m.Attempts()+1 {
			t.Fatalf("to send: %+v", b.pending)
		}
		b, _, _ = bounced(onfail.Escalate{To: other}, max, false)
		if b.dropped != 1 || len(b.pending) != 0 {
			t.Fatalf("at %d attempts: %d dropped, %d to send", max, b.dropped, len(b.pending))
		}
	})
	t.Run("escalate gives it to the other engine, and not while stopping", func(t *testing.T) {
		b, _, _ := bounced(onfail.Escalate{To: other}, 0, false)
		if len(b.pending) != 1 || b.pending[0].Dest() != other {
			t.Fatalf("to send: %+v", b.pending)
		}
		b, _, _ = bounced(onfail.Escalate{To: other}, 0, true)
		if b.dropped != 1 || len(b.pending) != 0 {
			t.Fatalf("while stopping: %d dropped, %d to send", b.dropped, len(b.pending))
		}
	})
	t.Run("compensate sends the message that undoes the first", func(t *testing.T) {
		b, _, _ := bounced(onfail.Compensate{With: undo}, 0, false)
		if len(b.pending) != 1 || b.pending[0] != undo {
			t.Fatalf("to send: %+v", b.pending)
		}
	})
	t.Run("notify tells the actor of the message", func(t *testing.T) {
		_, told, _ := bounced(onfail.Notify{Key: "gone", Args: []string{"a", "b"}}, 0, false)
		want := []notice{{actor: any(state.CharacterID(5)), key: "gone", args: []any{"a", "b"}}}
		if !reflect.DeepEqual(told, want) {
			t.Fatalf("told %+v, want %+v", told, want)
		}
	})
	t.Run("a message with no OnFail is dropped", func(t *testing.T) {
		b, _, _ := bounced(nil, 0, false)
		if b.dropped != 1 || len(b.pending) != 0 {
			t.Fatalf("%d dropped, %d to send", b.dropped, len(b.pending))
		}
	})
}
