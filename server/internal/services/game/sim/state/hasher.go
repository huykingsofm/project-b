package state

import "math/bits"

// Hasher builds a number that depends on the values written to it and on nothing else. It is meant
// to be fast, not strong: it takes a whole 64-bit word in one step (a rotate, an xor and a
// multiply, as FxHash does) and mixes the result once at the end, which is enough to tell two
// states apart in a replay. It makes no allocation. A state uses it to say what its data is.
type Hasher uint64

const hasherMul = 0x517cc1b727220a95

// NewHasher returns a hasher that has had nothing written to it.
func NewHasher() Hasher { return 0 }

// U64 writes a number.
func (h *Hasher) U64(v uint64) {
	*h = Hasher((bits.RotateLeft64(uint64(*h), 5) ^ v) * hasherMul)
}

// String writes a text, eight bytes at a time, with its length so that two texts in a row cannot
// run together.
func (h *Hasher) String(s string) {
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

// Sum returns the number, after one last mixing of its bits.
func (h Hasher) Sum() uint64 {
	x := uint64(h)
	x ^= x >> 32
	x *= 0xd6e8feb86659fd93
	x ^= x >> 32
	return x
}
