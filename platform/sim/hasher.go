package sim

import (
	"fmt"
	"math/bits"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

const hasherMul = 0x517cc1b727220a95

// Hasher builds a number that depends on the values written to it and on nothing else. It is meant
// to be fast, not strong: it takes a whole 64-bit word in one step (a rotate, an xor and a
// multiply, as FxHash does) and mixes the result once at the end, which is enough to tell two
// states apart in a replay. It makes no allocation. A state uses it to say what its data is.
type Hasher uint64

// NewHasher returns a hasher that has had nothing written to it.
func NewHasher() Hasher { return 0 }

// U64 writes a number.
func (h *Hasher) U64(v uint64) {
	*h = Hasher((bits.RotateLeft64(uint64(*h), 5) ^ v) * hasherMul)
}

// String writes a text, eight bytes at a time, with its length so that two texts in a row cannot
// run together.
func (h *Hasher) String(s string) { hashBytes(h, s) }

// Bytes writes bytes, as String writes a text.
func (h *Hasher) Bytes(b []byte) { hashBytes(h, b) }

// Sum returns the number, after one last mixing of its bits.
func (h Hasher) Sum() uint64 {
	x := uint64(h)
	x ^= x >> 32
	x *= 0xd6e8feb86659fd93
	x ^= x >> 32
	return x
}

// Actions adds a list of actions to the hash: each as the bytes it is written as, which say
// everything it holds, the parts that only it knows included. An action that cannot be written adds
// its type, so the hash still tells it from another.
func (h *Hasher) Actions[S State](actions []Action[S]) {
	var w binary.Writer
	h.actionsWith(actions, &w)
}

// actionsWith is Actions with the bytes to write each action to, which a caller that hashes
// many lists makes once.
func (h *Hasher) actionsWith[S State](actions []Action[S], w *binary.Writer) {
	h.U64(uint64(len(actions)))
	for _, a := range actions {
		w.B = w.B[:0]
		if EncodeAction(w, a) != nil {
			h.String(fmt.Sprintf("%T", a))
			continue
		}
		h.Bytes(w.B)
	}
}

func hashBytes[T string | []byte](h *Hasher, s T) {
	h.U64(uint64(len(s)))
	for len(s) >= 8 {
		h.U64(uint64(s[0]) | uint64(s[1])<<8 | uint64(s[2])<<16 | uint64(s[3])<<24 |
			uint64(s[4])<<32 | uint64(s[5])<<40 | uint64(s[6])<<48 | uint64(s[7])<<56)
		s = s[8:]
	}
	var tail uint64
	for i := 0; i < len(s); i++ {
		tail |= uint64(s[i]) << (8 * i)
	}
	h.U64(tail)
}
