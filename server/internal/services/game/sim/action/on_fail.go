package action

import (
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

// OnFailID names a kind of OnFail, by a number: see the register below. A kind returns its own with
// its method ID(). A number is never changed or reused, because the replay log writes it.
type OnFailID uint16

// The IDs of the kinds of OnFail. The numbers count up from 1 and 0 is no kind.
const (
	OnFailRetryID OnFailID = iota + 1
	OnFailRedirectID
	OnFailCompensateID
	OnFailNotifyID
	OnFailEscalateID
	OnFailDropID
)

// OnFail is the part of a message that says what a failed delivery means. Each kind is a type of
// its own that holds what it needs, runs itself on the engine that sent the message (Handle), and
// writes itself (Encode); a function in its codec reads it back, found by its ID. A message is
// written whole, so an OnFail is data and never a function. Every send names one: a message with
// none is dropped, and logged.
type OnFail interface {
	ID() OnFailID
	// Handle does what a failed delivery means, given the engine that sent the message, the
	// message and why it could not be delivered.
	Handle(b Bouncer, m Message, r Reason)
	// Encode writes what the kind holds, not its ID: EncodeOnFail writes the ID first.
	Encode(w *binarycodec.Writer) error
}

// Bouncer is what an OnFail may ask of the engine that sent a message that could not be delivered.
type Bouncer interface {
	// Resend makes the engine send the message at its next flush.
	Resend(m Message)
	// Tell tells an actor something, as a text key with arguments. An actor that is nil is told
	// nothing.
	Tell(actor any, key string, args []string)
	// Drop forgets a message and counts it.
	Drop()
	// Stopping reports whether the engine is stopping, when nothing is sent again.
	Stopping() bool
	// MaxAttempts is how many times a message is tried before it is dropped.
	MaxAttempts() int
}

// OnFailRetry sends the message again to the same address.
type OnFailRetry struct{}

func (OnFailRetry) ID() OnFailID { return OnFailRetryID }

// Handle sends it again, unless the engine stops or the message was tried too often.
func (OnFailRetry) Handle(b Bouncer, m Message, r Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Again())
	}
}

func (OnFailRetry) Encode(*binarycodec.Writer) error { return nil }

func decodeOnFailRetry(*binarycodec.Reader, binarycodec.Version, int) (OnFail, error) {
	return OnFailRetry{}, nil
}

// OnFailRedirect sends the message to another engine of the same scope.
type OnFailRedirect struct{ To Address }

func (OnFailRedirect) ID() OnFailID { return OnFailRedirectID }

// Handle sends it, once more tried, to the other engine, unless the engine stops or the message was
// tried too often.
func (f OnFailRedirect) Handle(b Bouncer, m Message, r Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

func (f OnFailRedirect) Encode(w *binarycodec.Writer) error {
	f.To.Encode(w)
	return nil
}

func decodeOnFailRedirect(
	r *binarycodec.Reader, version binarycodec.Version, _ int,
) (OnFail, error) {
	return OnFailRedirect{To: DecodeAddress(r, version)}, r.Err()
}

// OnFailCompensate sends the message that undoes what the first one set going: where it goes, its
// actions and its own OnFail. The chain ends at a kind that holds no message.
type OnFailCompensate struct{ With Message }

func (OnFailCompensate) ID() OnFailID { return OnFailCompensateID }

// Handle sends the message that undoes the first.
func (f OnFailCompensate) Handle(b Bouncer, _ Message, _ Reason) { b.Resend(f.With) }

func (f OnFailCompensate) Encode(w *binarycodec.Writer) error { return EncodeMessage(w, f.With) }

func decodeOnFailCompensate(
	r *binarycodec.Reader, version binarycodec.Version, depth int,
) (OnFail, error) {
	m, err := decodeMessage(r, version, depth+1)
	return OnFailCompensate{With: m}, err
}

// OnFailNotify tells the actor that the message is about, with a text key and its arguments.
type OnFailNotify struct {
	Key  string
	Args []string
}

func (OnFailNotify) ID() OnFailID { return OnFailNotifyID }

// Handle tells the actor of the message.
func (f OnFailNotify) Handle(b Bouncer, m Message, _ Reason) { b.Tell(m.Subject(), f.Key, f.Args) }

func (f OnFailNotify) Encode(w *binarycodec.Writer) error {
	w.String(f.Key)
	w.Uvarint(uint64(len(f.Args)))
	for _, a := range f.Args {
		w.String(a)
	}
	return nil
}

func decodeOnFailNotify(
	r *binarycodec.Reader, _ binarycodec.Version, _ int,
) (OnFail, error) {
	f := OnFailNotify{Key: r.String()}
	n := r.Uvarint()
	if n > uint64(r.Left()) { // a string takes at least one byte
		return nil, binarycodec.ErrShort
	}
	for range n {
		f.Args = append(f.Args, r.String())
	}
	return f, r.Err()
}

// OnFailEscalate gives the message to the engine at the address to decide.
type OnFailEscalate struct{ To Address }

func (OnFailEscalate) ID() OnFailID { return OnFailEscalateID }

// Handle gives it to the other engine, unless the engine stops.
func (f OnFailEscalate) Handle(b Bouncer, m Message, _ Reason) {
	if b.Stopping() {
		b.Drop()
		return
	}
	b.Resend(m.Redirected(f.To))
}

func (f OnFailEscalate) Encode(w *binarycodec.Writer) error {
	f.To.Encode(w)
	return nil
}

func decodeOnFailEscalate(
	r *binarycodec.Reader, version binarycodec.Version, _ int,
) (OnFail, error) {
	return OnFailEscalate{To: DecodeAddress(r, version)}, r.Err()
}

// OnFailDrop forgets the message and counts it.
type OnFailDrop struct{}

func (OnFailDrop) ID() OnFailID { return OnFailDropID }

// Handle drops the message.
func (OnFailDrop) Handle(b Bouncer, _ Message, _ Reason) { b.Drop() }

func (OnFailDrop) Encode(*binarycodec.Writer) error { return nil }

func decodeOnFailDrop(*binarycodec.Reader, binarycodec.Version, int) (OnFail, error) {
	return OnFailDrop{}, nil
}

// resendable reports whether a message that is to be sent again may be: not while the engine stops,
// and not once it has been tried the most that it may. When it may not, it is dropped.
func resendable(b Bouncer, m Message, r Reason) bool {
	if b.Stopping() {
		b.Drop()
		return false
	}
	if m.Attempts() >= b.MaxAttempts() {
		logging.Warn(
			"message dropped: too many attempts",
			"from", m.Head().From, "dest", m.Dest(), "reason", r, "attempts", m.Attempts(),
			"max_attempts", b.MaxAttempts(),
		)
		b.Drop()
		return false
	}
	return true
}
